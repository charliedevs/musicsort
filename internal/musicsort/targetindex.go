package musicsort

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"musicsort/internal/audio"
	"musicsort/internal/musicmatch"
)

// layoutSchema describes which directory levels of a Layout correspond to
// the artist and album parts. It lets the TargetIndex walk the target tree
// without carrying any layout-specific knowledge: an artist folder is at
// depth `artistDepth`, the album folder under it at depth `albumDepth`,
// regardless of whether there's a genre folder above.
type layoutSchema struct {
	artistDepth int // -1 if absent
	albumDepth  int
	totalDepth  int // total component count
}

// schemaFor inspects a layout's Components function (with an empty
// FileMetadata) to figure out the part ordering. Layouts whose Components
// fn always returns the same number of entries with the same kinds are the
// only ones supported here, which is true of every preset shipped with
// musicsort.
func schemaFor(l *Layout) layoutSchema {
	comps := l.Components(FileMetadata{})
	s := layoutSchema{artistDepth: -1, albumDepth: -1, totalDepth: len(comps)}
	for i, c := range comps {
		switch c.Kind {
		case PartKindArtist:
			s.artistDepth = i
		case PartKindAlbum:
			s.albumDepth = i
		}
	}
	return s
}

// AlbumFolder is a single album-level directory under an artist.
type AlbumFolder struct {
	// OnDisk is the actual folder name as it appears in the target tree,
	// e.g. "Faces (2014)" or "FACES (2014)". Two AlbumFolders sharing the
	// same NormalizeForMatch key are duplicate-edition variants the
	// consolidation pass will merge.
	OnDisk string
	// AbsPath is the absolute path on disk.
	AbsPath string
	// FileCount is the number of audio files directly inside this folder,
	// used to pick a canonical winner during consolidation.
	FileCount int
}

// ArtistFolder is a top-level artist directory containing some number of
// albums (or, equivalently, however many directories the active layout
// puts at the album depth).
type ArtistFolder struct {
	OnDisk  string
	AbsPath string
	// Albums groups album-level folders by NormalizeForMatch(album_name)
	// with the trailing "(YYYY)" year suffix already stripped, so an
	// album with year "Faces (2014)" and one without "Faces" collapse to
	// the same key.
	Albums map[string][]*AlbumFolder
	// FileCount is the total number of audio files anywhere under this
	// artist folder, summed from each AlbumFolder plus loose files at
	// the artist level (e.g. when the user dropped an unsorted single
	// directly into the artist dir).
	FileCount int
}

// TargetIndex captures the existing folder structure of a target directory
// so the organizer can route new files to canonical folders and merge
// pre-existing case/edition duplicates.
type TargetIndex struct {
	targetDir string
	layout    *Layout
	schema    layoutSchema

	// artists is keyed on NormalizeForMatch(folder name). Each key may
	// contain multiple ArtistFolder entries (case-fold or
	// diacritic-fold dupes); the consolidation pass merges them. For
	// layouts without a PartKindArtist component (e.g. flat) the map
	// stays empty.
	artists map[string][]*ArtistFolder
}

// newTargetIndex returns a TargetIndex bound to the given target directory
// and layout. Call BuildTargetIndex (the package-level convenience) to also
// scan the directory.
func newTargetIndex(targetDir string, layout *Layout) *TargetIndex {
	return &TargetIndex{
		targetDir: targetDir,
		layout:    layout,
		schema:    schemaFor(layout),
		artists:   make(map[string][]*ArtistFolder),
	}
}

// BuildTargetIndex scans the target directory and returns a populated
// TargetIndex. It is safe to call on a missing or empty target dir; the
// returned index is empty in that case.
func BuildTargetIndex(targetDir string, layout *Layout) (*TargetIndex, error) {
	idx := newTargetIndex(targetDir, layout)
	if idx.schema.artistDepth < 0 {
		// Layouts with no artist component (flat) skip the scan: there
		// are no folders to dedup, only filename collisions, which are
		// handled at write time.
		return idx, nil
	}
	if _, err := os.Stat(targetDir); err != nil {
		if os.IsNotExist(err) {
			return idx, nil
		}
		return nil, err
	}

	err := filepath.WalkDir(targetDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable subtrees rather than failing the whole run
		}
		rel, _ := filepath.Rel(targetDir, path)
		if rel == "." {
			return nil
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		depth := len(parts) - 1 // 0-based: parts[0] is at depth 0

		if d.IsDir() {
			switch {
			case depth == idx.schema.artistDepth:
				idx.recordArtist(parts[depth], path)
			case depth == idx.schema.albumDepth:
				idx.recordAlbum(parts, path)
			case depth > idx.schema.albumDepth:
				// We don't index any directory levels beyond album.
				return filepath.SkipDir
			}
			return nil
		}
		// Files: count them against whichever folder they live in.
		if !audio.SupportedExtensions[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		idx.attributeFile(parts)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return idx, nil
}

// recordArtist creates the ArtistFolder bucket for an artist-level
// directory. Multiple physical folders that normalize to the same key
// produce multiple bucket entries; consolidation later picks a winner.
func (idx *TargetIndex) recordArtist(name, abs string) {
	key := musicmatch.NormalizeForMatch(name)
	if key == "" {
		return
	}
	af := &ArtistFolder{
		OnDisk:  name,
		AbsPath: abs,
		Albums:  make(map[string][]*AlbumFolder),
	}
	idx.artists[key] = append(idx.artists[key], af)
}

// recordAlbum attaches an album-level directory to its parent artist
// bucket. If the artist somehow wasn't recorded first (rare; would only
// happen if the artist directory was unreadable) we silently skip.
func (idx *TargetIndex) recordAlbum(parts []string, abs string) {
	if idx.schema.artistDepth < 0 || idx.schema.artistDepth >= len(parts) {
		return
	}
	parent := idx.firstArtistByName(parts[idx.schema.artistDepth])
	if parent == nil {
		return
	}
	albumName := parts[idx.schema.albumDepth]
	key := albumDedupKey(albumName)
	if key == "" {
		return
	}
	af := &AlbumFolder{OnDisk: albumName, AbsPath: abs}
	parent.Albums[key] = append(parent.Albums[key], af)
}

// attributeFile increments the file counter on whichever folder bucket the
// file is logically attached to. Used by the consolidation pass to pick the
// canonical winner among case-fold dupes.
func (idx *TargetIndex) attributeFile(parts []string) {
	if idx.schema.artistDepth < 0 || idx.schema.artistDepth >= len(parts) {
		return
	}
	parent := idx.firstArtistByName(parts[idx.schema.artistDepth])
	if parent == nil {
		return
	}
	parent.FileCount++
	if idx.schema.albumDepth < 0 || idx.schema.albumDepth >= len(parts) {
		return
	}
	album := idx.firstAlbumByName(parent, parts[idx.schema.albumDepth])
	if album == nil {
		return
	}
	album.FileCount++
}

// firstArtistByName returns the ArtistFolder whose OnDisk name matches name
// (the same physical directory we recorded earlier). Used while walking
// files so attributeFile can find the parent it just recorded.
func (idx *TargetIndex) firstArtistByName(name string) *ArtistFolder {
	key := musicmatch.NormalizeForMatch(name)
	for _, af := range idx.artists[key] {
		if af.OnDisk == name {
			return af
		}
	}
	return nil
}

// firstAlbumByName returns the AlbumFolder under parent whose OnDisk name
// matches name. Mirrors firstArtistByName for the album level.
func (idx *TargetIndex) firstAlbumByName(parent *ArtistFolder, name string) *AlbumFolder {
	key := albumDedupKey(name)
	for _, ab := range parent.Albums[key] {
		if ab.OnDisk == name {
			return ab
		}
	}
	return nil
}

// albumDedupKey is the dedup-key form of an album folder name: the trailing
// "(YYYY)" year suffix is removed before normalization so a 2014 "Faces"
// reissue and a year-less "Faces" tag collapse to one entry. Exposed
// (lowercased name) for tests.
func albumDedupKey(name string) string {
	stripped := musicmatch.AlbumYearSuffix.ReplaceAllString(name, "")
	return musicmatch.NormalizeForMatch(stripped)
}

// ResolveArtist returns the canonical on-disk artist folder name for an
// incoming file with the given raw artist tag. If a matching folder
// already exists in the target tree, its existing casing is reused so the
// new track lands in the existing folder rather than creating a
// case-fold sibling. Returns "" only when the input is empty or the
// layout has no artist component.
func (idx *TargetIndex) ResolveArtist(rawArtist string) string {
	if idx.schema.artistDepth < 0 {
		return ""
	}
	key := musicmatch.NormalizeForMatch(rawArtist)
	if key == "" {
		return Sanitize(rawArtist)
	}
	if winner := idx.canonicalArtist(key); winner != nil {
		return winner.OnDisk
	}
	return Sanitize(rawArtist)
}

// ResolveAlbum returns the canonical on-disk album folder name for an
// incoming file. Falls back to Sanitize(rawAlbum)+suffix when no existing
// folder matches.
func (idx *TargetIndex) ResolveAlbum(rawArtist, rawAlbum, suffix string) string {
	if idx.schema.albumDepth < 0 {
		return ""
	}
	defaultName := Sanitize(rawAlbum) + suffix

	artistKey := musicmatch.NormalizeForMatch(rawArtist)
	winnerArtist := idx.canonicalArtist(artistKey)
	if winnerArtist == nil {
		return defaultName
	}
	albumKey := musicmatch.NormalizeForMatch(rawAlbum)
	if albumKey == "" {
		return defaultName
	}
	if winner := canonicalAlbum(winnerArtist, albumKey); winner != nil {
		return winner.OnDisk
	}
	return defaultName
}

// RegisterIncoming records that a file is now (logically) routed to the
// given artist+album folder names, so subsequent files in the same run
// route to the same canonical names even if their tags differ in casing.
// This handles the in-process case where the target dir starts empty and
// two files with case-variant artist tags arrive back-to-back.
func (idx *TargetIndex) RegisterIncoming(artistName, albumName string) {
	if idx.schema.artistDepth < 0 {
		return
	}
	if name := strings.TrimSpace(artistName); name != "" {
		key := musicmatch.NormalizeForMatch(name)
		if existing := idx.canonicalArtist(key); existing == nil {
			af := &ArtistFolder{
				OnDisk:  artistName,
				AbsPath: filepath.Join(idx.targetDir, artistName),
				Albums:  make(map[string][]*AlbumFolder),
			}
			idx.artists[key] = append(idx.artists[key], af)
		}
	}
	if albumName == "" || idx.schema.albumDepth < 0 {
		return
	}
	parent := idx.canonicalArtist(musicmatch.NormalizeForMatch(artistName))
	if parent == nil {
		return
	}
	key := albumDedupKey(albumName)
	if key == "" {
		return
	}
	if canonicalAlbum(parent, key) == nil {
		parent.Albums[key] = append(parent.Albums[key], &AlbumFolder{
			OnDisk:  albumName,
			AbsPath: filepath.Join(parent.AbsPath, albumName),
		})
	}
}

// canonicalArtist returns the canonical winner among the ArtistFolder
// entries sharing key. Picks: highest FileCount, then longest OnDisk name
// (preserving informative casings/punctuation), then alphabetically. Used
// both during dedup-aware lookup and by the step-5 consolidation pass to
// agree on the same winner.
func (idx *TargetIndex) canonicalArtist(key string) *ArtistFolder {
	bucket := idx.artists[key]
	return pickCanonicalArtist(bucket)
}

func pickCanonicalArtist(bucket []*ArtistFolder) *ArtistFolder {
	if len(bucket) == 0 {
		return nil
	}
	if len(bucket) == 1 {
		return bucket[0]
	}
	sorted := append([]*ArtistFolder(nil), bucket...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return betterCanonicalName(sorted[i].FileCount, sorted[i].OnDisk,
			sorted[j].FileCount, sorted[j].OnDisk)
	})
	return sorted[0]
}

// canonicalAlbum mirrors canonicalArtist for the album level under a given
// artist. The longer-name preference matters here: when "Faces (2014)" and
// "Faces (2021)" both exist, we keep whichever has more files.
func canonicalAlbum(parent *ArtistFolder, key string) *AlbumFolder {
	bucket := parent.Albums[key]
	return pickCanonicalAlbum(bucket)
}

func pickCanonicalAlbum(bucket []*AlbumFolder) *AlbumFolder {
	if len(bucket) == 0 {
		return nil
	}
	if len(bucket) == 1 {
		return bucket[0]
	}
	sorted := append([]*AlbumFolder(nil), bucket...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return betterCanonicalName(sorted[i].FileCount, sorted[i].OnDisk,
			sorted[j].FileCount, sorted[j].OnDisk)
	})
	return sorted[0]
}

// betterCanonicalName returns true if (countA, nameA) should rank higher
// than (countB, nameB) when picking a canonical winner among case-fold
// duplicates. The ordering is:
//
//  1. More files wins (consolidating into the bigger bucket avoids the
//     most disk I/O and surfaces the artist/album the user has played
//     into the most).
//  2. Mixed-case beats ALL-UPPERCASE. This matters for the real-world
//     "$uicideboy$" vs "$UICIDEBOY$" / "Faces" vs "FACES" cases the
//     README TODO calls out: the screaming-uppercase variant is almost
//     always the "wrong" one a careless tagger inserted, and the
//     mixed-case variant matches what the artist actually intends.
//  3. Longer name (preserves edition annotations like "(Deluxe)").
//  4. Alphabetical for fully-deterministic output across runs.
func betterCanonicalName(countA int, nameA string, countB int, nameB string) bool {
	if countA != countB {
		return countA > countB
	}
	allCapsA := isAllCaps(nameA)
	allCapsB := isAllCaps(nameB)
	if allCapsA != allCapsB {
		return !allCapsA
	}
	if len(nameA) != len(nameB) {
		return len(nameA) > len(nameB)
	}
	return nameA < nameB
}

// isAllCaps reports whether name has at least one ASCII letter and zero
// lowercase ASCII letters. Names with no letters at all (e.g. "30/70")
// return false because there's no case to compare.
func isAllCaps(name string) bool {
	hasLetter, hasLower := false, false
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			hasLetter, hasLower = true, true
		case r >= 'A' && r <= 'Z':
			hasLetter = true
		}
	}
	return hasLetter && !hasLower
}

// ArtistGroups returns the artist buckets grouped by normalization key, in
// deterministic key order. The consolidation pass iterates this to find
// duplicate buckets (len(bucket) > 1) and merge them.
func (idx *TargetIndex) ArtistGroups() []ArtistGroup {
	keys := make([]string, 0, len(idx.artists))
	for k := range idx.artists {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	groups := make([]ArtistGroup, 0, len(keys))
	for _, k := range keys {
		groups = append(groups, ArtistGroup{Key: k, Folders: idx.artists[k]})
	}
	return groups
}

// ArtistGroup pairs a normalized key with the bucket of ArtistFolders that
// share it.
type ArtistGroup struct {
	Key     string
	Folders []*ArtistFolder
}
