package musicsort

import (
	"crypto/sha1"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"musicsort/internal/clioutput"
)

// firstMBSampleSize is the number of leading bytes we hash when deciding
// whether two files with the same name and size are actually the same
// content. A full-file hash on every collision would be wasteful: most
// audio files differ in their first frame already (different bitrates,
// different leading silence, different artwork blobs in the tag), so a
// 1MB sample is enough to distinguish "bit-identical re-download" from
// "two different recordings of the same track".
const firstMBSampleSize = 1 << 20 // 1 MiB

// consolidateExisting walks every artist bucket in the TargetIndex, picks a
// canonical winner per case-fold group, and merges every other "loser"
// folder into the winner. Each file move uses the same MoveFile helper as
// the regular organize step so cross-device fallback still works.
//
// The pass is intentionally conservative:
//
//   - Only fires when there are at least two ArtistFolder entries that
//     share a normalized key (same for AlbumFolder).
//   - Filename collisions in the winner trigger a content check first
//     (size + first-MB sha1); identical bytes are dropped as duplicates,
//     non-identical bytes are renamed with " (2)" / " (3)" suffixes.
//   - Honors --dry-run: it logs everything it would do without touching
//     the filesystem.
func (o *TrackOrganizer) consolidateExisting() error {
	if o.index == nil {
		return nil
	}
	for _, group := range o.index.ArtistGroups() {
		if len(group.Folders) > 1 {
			if err := o.mergeArtistGroup(group); err != nil {
				return err
			}
		}
		// Even if the artist group itself is unique, individual album
		// buckets inside the (single) winner can still hold edition
		// dupes — collapse them too.
		winner := pickCanonicalArtist(group.Folders)
		if winner == nil {
			continue
		}
		if err := o.mergeAlbumDupesUnder(winner); err != nil {
			return err
		}
	}
	return nil
}

// mergeArtistGroup merges every loser ArtistFolder in a bucket into the
// canonical winner. For each loser, every album beneath it is folded into
// the winner's matching album bucket (or moved as-is when the winner has
// no equivalent).
func (o *TrackOrganizer) mergeArtistGroup(group ArtistGroup) error {
	winner := pickCanonicalArtist(group.Folders)
	if winner == nil {
		return nil
	}

	for _, loser := range group.Folders {
		if loser == winner {
			continue
		}
		o.logConsolidate("artist", loser.OnDisk, winner.OnDisk)

		// Move each album folder under the loser into the winner, one
		// at a time. We pre-snapshot the loser's album entries because
		// the index mutates as we move things.
		for _, bucket := range loser.Albums {
			for _, ab := range bucket {
				if err := o.mergeAlbumIntoArtist(ab, winner); err != nil {
					return err
				}
			}
		}

		// Move loose audio files (and any non-audio companions) directly
		// under the loser artist dir into the winner artist dir, then
		// remove the now-empty loser.
		if err := o.mergeLooseFiles(loser.AbsPath, winner.AbsPath); err != nil {
			return err
		}
		if err := o.removeIfEmpty(loser.AbsPath); err != nil {
			return err
		}

		o.addConsolidated(1)
	}

	// Re-collapse the bucket so subsequent index lookups (during the
	// regular organize pass) see only the winner.
	o.index.artists[group.Key] = []*ArtistFolder{winner}
	return nil
}

// mergeAlbumDupesUnder collapses album-level duplicates within a single
// (winner) artist folder. Used both directly (when an artist bucket only
// has one entry but its album buckets have dupes) and as part of the
// post-mergeArtistGroup pass.
func (o *TrackOrganizer) mergeAlbumDupesUnder(parent *ArtistFolder) error {
	for key, bucket := range parent.Albums {
		if len(bucket) <= 1 {
			continue
		}
		winner := pickCanonicalAlbum(bucket)
		if winner == nil {
			continue
		}
		for _, loser := range bucket {
			if loser == winner {
				continue
			}
			o.logConsolidate("album", loser.OnDisk, winner.OnDisk)
			if err := o.mergeFiles(loser.AbsPath, winner.AbsPath); err != nil {
				return err
			}
			if err := o.removeIfEmpty(loser.AbsPath); err != nil {
				return err
			}
			o.addConsolidated(1)
		}
		parent.Albums[key] = []*AlbumFolder{winner}
	}
	return nil
}

// mergeAlbumIntoArtist takes one album folder under a loser-artist and
// places its contents under the winner-artist. If the winner already has
// an album folder with the same dedup key, we recurse into mergeFiles to
// merge file-by-file; otherwise we move the directory wholesale.
func (o *TrackOrganizer) mergeAlbumIntoArtist(loser *AlbumFolder, winner *ArtistFolder) error {
	key := albumDedupKey(loser.OnDisk)
	if existing := canonicalAlbum(winner, key); existing != nil {
		o.logConsolidate("album", loser.OnDisk, existing.OnDisk)
		if err := o.mergeFiles(loser.AbsPath, existing.AbsPath); err != nil {
			return err
		}
		if err := o.removeIfEmpty(loser.AbsPath); err != nil {
			return err
		}
		o.addConsolidated(1)
		return nil
	}

	// Winner has no equivalent album: re-home the whole folder. We treat
	// the operation as "moving" not "consolidating" since no merge of
	// pre-existing folders is happening.
	dest := filepath.Join(winner.AbsPath, loser.OnDisk)
	if o.dryRun {
		clioutput.InfoLine("%s %s -> %s",
			clioutput.Label("DRY-RUN MOVE", clioutput.Yellow),
			loser.AbsPath, dest)
		return nil
	}
	if err := CreateDirectoryPath(winner.AbsPath); err != nil {
		return err
	}
	if err := os.Rename(loser.AbsPath, dest); err != nil {
		// Cross-device or non-empty destination: fall back to the
		// per-file path so we still make progress.
		return o.mergeFiles(loser.AbsPath, dest)
	}
	winner.Albums[key] = append(winner.Albums[key], &AlbumFolder{
		OnDisk:    loser.OnDisk,
		AbsPath:   dest,
		FileCount: loser.FileCount,
	})
	return nil
}

// mergeLooseFiles moves every audio file directly inside src into dst,
// using the same collision policy as mergeFiles. Used to relocate stray
// singles that were dropped at the artist level rather than inside an
// album folder.
func (o *TrackOrganizer) mergeLooseFiles(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if err := o.mergeOneFile(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// mergeFiles walks every file under src and routes it into dst, mirroring
// any subdirectory structure it finds (so an album folder can be merged
// without losing inner artwork or bonus-disc subdirs).
func (o *TrackOrganizer) mergeFiles(src, dst string) error {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil
	}
	if !o.dryRun {
		if err := CreateDirectoryPath(dst); err != nil {
			return err
		}
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		srcPath := filepath.Join(src, e.Name())
		dstPath := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := o.mergeFiles(srcPath, dstPath); err != nil {
				return err
			}
			if err := o.removeIfEmpty(srcPath); err != nil {
				return err
			}
			continue
		}
		if err := o.mergeOneFile(srcPath, dstPath); err != nil {
			return err
		}
	}
	return nil
}

// mergeOneFile routes a single file from src to dst, deciding between
// "skip duplicate" and "rename and keep" based on a content fingerprint
// when dst already exists.
func (o *TrackOrganizer) mergeOneFile(src, dst string) error {
	if !FileExists(dst) {
		return o.moveLogged(src, dst, "MERGE")
	}
	dup, err := sameContent(src, dst)
	if err != nil {
		return err
	}
	if dup {
		if o.dryRun {
			if o.verbose {
				clioutput.InfoLine("%s %s (duplicate of %s)",
					clioutput.Label("DRY-RUN SKIP", clioutput.Yellow), src, dst)
			}
			return nil
		}
		if o.verbose {
			PrintSkip(filepath.Base(src), "duplicate of "+dst)
		}
		if err := os.Remove(src); err != nil {
			return fmt.Errorf("remove duplicate %s: %w", src, err)
		}
		o.addResult(0, 1, 0)
		return nil
	}
	// Different bytes: rename to a non-conflicting destination.
	renamed := uniquePath(dst)
	if o.dryRun {
		if o.verbose {
			clioutput.InfoLine("%s %s -> %s (collision; renamed)",
				clioutput.Label("DRY-RUN RENAME", clioutput.Yellow), src, renamed)
		}
		o.addRenamedOnMerge(1)
		return nil
	}
	if o.verbose {
		clioutput.InfoLine("%s %s -> %s",
			clioutput.Label("RENAME", clioutput.Yellow), src, renamed)
	}
	if err := MoveFile(src, renamed, false); err != nil {
		return err
	}
	o.addRenamedOnMerge(1)
	return nil
}

// moveLogged is mergeOneFile's no-collision path, factored out so verbose
// logging stays centralized.
func (o *TrackOrganizer) moveLogged(src, dst, label string) error {
	if o.dryRun {
		if o.verbose {
			clioutput.InfoLine("%s %s -> %s",
				clioutput.Label("DRY-RUN "+label, clioutput.Yellow), src, dst)
		}
		return nil
	}
	if err := CreateDirectoryPath(filepath.Dir(dst)); err != nil {
		return err
	}
	if o.verbose {
		clioutput.InfoLine("%s %s -> %s",
			clioutput.Label(label, clioutput.Green), src, dst)
	}
	return MoveFile(src, dst, false)
}

// removeIfEmpty removes dir if it's empty (and the run isn't dry). Silently
// ignores non-empty / missing directories so callers can use it as a
// best-effort cleanup step.
func (o *TrackOrganizer) removeIfEmpty(dir string) error {
	if o.dryRun {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(entries) > 0 {
		return nil
	}
	if err := os.Remove(dir); err != nil {
		return fmt.Errorf("remove empty dir %s: %w", dir, err)
	}
	if o.verbose {
		clioutput.InfoLine("%s %s",
			clioutput.Label("REMOVE", clioutput.Yellow), dir)
	}
	return nil
}

// logConsolidate is the per-folder log line emitted when we plan or
// perform a merge. Keeps the wording consistent across artist and album
// merges.
func (o *TrackOrganizer) logConsolidate(level, loser, winner string) {
	label := "CONSOLIDATE"
	if o.dryRun {
		label = "DRY-RUN " + label
	}
	if o.verbose || o.dryRun {
		clioutput.InfoLine("%s %s: %q -> %q",
			clioutput.Label(label, clioutput.Cyan), level, loser, winner)
	}
}

// addConsolidated bumps Result.Consolidated under the result mutex so
// concurrent organize-time updates don't race the counter.
func (o *TrackOrganizer) addConsolidated(n int) {
	o.resultMutex.Lock()
	o.result.Consolidated += n
	o.resultMutex.Unlock()
}

// addRenamedOnMerge bumps the rename-on-collision counter.
func (o *TrackOrganizer) addRenamedOnMerge(n int) {
	o.resultMutex.Lock()
	o.result.RenamedOnMerge += n
	o.resultMutex.Unlock()
}

// uniquePath finds the first non-existing variant of path by appending
// " (2)", " (3)"... before the extension.
func uniquePath(path string) string {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for i := 2; ; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, i, ext))
		if !FileExists(candidate) {
			return candidate
		}
	}
}

// sameContent reports whether two files are byte-for-byte identical for
// our purposes (size + sha1 of leading firstMBSampleSize bytes). Returns
// false on any read error: if we can't tell, we err on the side of keeping
// both copies (renaming) so no data is lost.
func sameContent(a, b string) (bool, error) {
	sa, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	sb, err := os.Stat(b)
	if err != nil {
		return false, err
	}
	if sa.Size() != sb.Size() {
		return false, nil
	}
	ha, err := samplePrefixHash(a)
	if err != nil {
		return false, nil
	}
	hb, err := samplePrefixHash(b)
	if err != nil {
		return false, nil
	}
	return ha == hb, nil
}

// samplePrefixHash returns the sha1 of the first firstMBSampleSize bytes
// of path (or the entire file if smaller).
func samplePrefixHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha1.New()
	limited := io.LimitReader(f, firstMBSampleSize)
	if _, err := io.Copy(h, limited); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
