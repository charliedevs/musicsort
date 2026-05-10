package spotifym3u

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"musicsort/internal/audio"
	"musicsort/internal/clioutput"
	"musicsort/internal/musicmatch"

	"github.com/dhowden/tag"
)

// localTrack is the in-memory representation of a single audio file in the
// source directory, with both its raw and normalized forms cached for fast
// lookup during matching.
type localTrack struct {
	Path         string
	Title        string
	Artists      []string
	Album        string
	Filename     string
	NormTitle    string
	NormArtists  []string
	NormAlbum    string
	NormFilename string
	TitleTokens  map[string]struct{}
}

// TrackIndex is the searchable representation of a source music directory.
// It keeps both a flat slice of every track (allTracks) and several
// pre-built hash maps that the matching stages key into directly.
type TrackIndex struct {
	rootDir     string
	exactMatch  map[string][]*localTrack
	titleArtist map[string][]*localTrack
	titleAlbum  map[string][]*localTrack
	artistOnly  map[string][]*localTrack
	titleOnly   map[string][]*localTrack
	allTracks   []*localTrack
}

// newTrackIndex returns a TrackIndex with all of its lookup maps initialized
// but no tracks yet.
func newTrackIndex(rootDir string) *TrackIndex {
	return &TrackIndex{
		rootDir:     rootDir,
		exactMatch:  make(map[string][]*localTrack),
		titleArtist: make(map[string][]*localTrack),
		titleAlbum:  make(map[string][]*localTrack),
		artistOnly:  make(map[string][]*localTrack),
		titleOnly:   make(map[string][]*localTrack),
	}
}

// BuildIndex walks rootDir for supported audio files and returns a
// TrackIndex populated with their normalized metadata. recursive selects
// between a single-directory scan and a full tree walk; verbose enables
// per-file logging.
func BuildIndex(rootDir string, recursive, verbose bool) (*TrackIndex, error) {
	index := newTrackIndex(rootDir)

	processed, skipped := 0, 0
	walkFn := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Surface walk errors (permissions, vanished files) but keep
			// going; an unreadable subtree shouldn't kill the entire run.
			if verbose {
				clioutput.InfoLine("%s %s: %v",
					clioutput.Label("WARN", clioutput.Yellow), path, err)
			}
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if !recursive && path != rootDir {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !audio.SupportedExtensions[ext] {
			return nil
		}

		track, err := buildTrack(rootDir, path)
		if err != nil {
			skipped++
			if verbose {
				rel, _ := filepath.Rel(rootDir, path)
				clioutput.InfoLine("%s %s: %v",
					clioutput.Label("SKIP", clioutput.Yellow), filepath.ToSlash(rel), err)
			}
			return nil
		}

		processed++
		if verbose {
			rel, _ := filepath.Rel(rootDir, path)
			clioutput.InfoLine("%s %s",
				clioutput.Label("INDEX", clioutput.Cyan), filepath.ToSlash(rel))
		} else {
			clioutput.ProgressLine("Indexing %d files...", processed)
		}

		index.addTrack(track)
		return nil
	}

	if err := filepath.WalkDir(rootDir, walkFn); err != nil {
		return nil, fmt.Errorf("walk source directory: %w", err)
	}

	if !verbose && clioutput.ProgressEnabled() {
		clioutput.Newline()
	}
	if verbose {
		clioutput.InfoLine("Indexed %d tracks (%d skipped)", processed, skipped)
	}

	return index, nil
}

// addTrack inserts t into the flat slice and every relevant hash bucket. The
// indexing rules are kept here (rather than inlined into BuildIndex's walk
// closure) so tests can exercise the exact same code path.
//
// Each artist gets its own row in the artist-keyed maps so a CSV row that
// names any one of a file's collaborators still hits the strong artist hash.
func (idx *TrackIndex) addTrack(t *localTrack) {
	idx.allTracks = append(idx.allTracks, t)
	if len(t.NormArtists) == 0 {
		k := musicmatch.BuildKey(t.NormTitle, "", t.NormAlbum)
		idx.exactMatch[k] = append(idx.exactMatch[k], t)
		ka := musicmatch.BuildKey(t.NormTitle, "", "")
		idx.titleArtist[ka] = append(idx.titleArtist[ka], t)
	} else {
		for _, na := range t.NormArtists {
			ke := musicmatch.BuildKey(t.NormTitle, na, t.NormAlbum)
			idx.exactMatch[ke] = append(idx.exactMatch[ke], t)
			kt := musicmatch.BuildKey(t.NormTitle, na, "")
			idx.titleArtist[kt] = append(idx.titleArtist[kt], t)
			idx.artistOnly[na] = append(idx.artistOnly[na], t)
		}
	}
	idx.titleAlbum[musicmatch.BuildKey(t.NormTitle, "", t.NormAlbum)] = append(
		idx.titleAlbum[musicmatch.BuildKey(t.NormTitle, "", t.NormAlbum)], t)
	idx.titleOnly[t.NormTitle] = append(idx.titleOnly[t.NormTitle], t)
}

// buildTrack reads metadata for a single audio file and returns a populated
// localTrack. It returns an error only when the file itself cannot be
// opened; an unreadable tag block falls through to filename and directory
// heuristics so a tagless file still produces a useful track entry.
func buildTrack(rootDir, path string) (*localTrack, error) {
	rel, err := filepath.Rel(rootDir, path)
	if err != nil {
		return nil, err
	}
	rel = filepath.ToSlash(rel)

	track := &localTrack{
		Path:         rel,
		Filename:     filepath.Base(path),
		NormFilename: musicmatch.NormalizeForMatch(filepath.Base(path)),
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if metadata, err := tag.ReadFrom(f); err == nil {
		applyTagMetadata(track, metadata)
	}
	applyFallbacks(track, rel)

	track.NormTitle = musicmatch.NormalizeForMatch(track.Title)
	track.NormArtists = normalizeArtists(track.Artists)
	track.NormAlbum = musicmatch.NormalizeForMatch(track.Album)
	track.TitleTokens = musicmatch.TokenSet(track.NormTitle)
	return track, nil
}

// applyTagMetadata copies the readable fields out of a tag.Metadata into
// track. It prefers AlbumArtist when present so compilation/feature tracks
// surface a stable primary artist, but also collects Artist() so
// collaborators are indexed alongside the lead.
func applyTagMetadata(track *localTrack, metadata tag.Metadata) {
	var artistFields []string
	if a := metadata.AlbumArtist(); a != "" {
		artistFields = append(artistFields, a)
	}
	if a := metadata.Artist(); a != "" && a != metadata.AlbumArtist() {
		artistFields = append(artistFields, a)
	}

	seen := make(map[string]struct{})
	var artists []string
	for _, field := range artistFields {
		for _, name := range musicmatch.SplitArtists(field) {
			key := strings.ToLower(name)
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			artists = append(artists, name)
		}
	}
	if len(artists) > 0 {
		track.Artists = artists
	}
	track.Album = metadata.Album()
	track.Title = metadata.Title()
}

// applyFallbacks fills in any missing track fields from the filename and
// parent directories. musicsort organizes files as
// "Artist/Album (Year)/Artist - Title.ext" so those are reliable signals
// when metadata is missing or unreadable.
func applyFallbacks(track *localTrack, rel string) {
	if track.Title == "" || len(track.Artists) == 0 {
		fnArtist, fnTitle := parseFilenameArtistTitle(track.Filename)
		if track.Title == "" {
			track.Title = fnTitle
		}
		if len(track.Artists) == 0 && fnArtist != "" {
			track.Artists = []string{fnArtist}
		}
		if len(track.Artists) == 0 {
			if dirArtist := parseDirectoryArtist(rel); dirArtist != "" {
				track.Artists = []string{dirArtist}
			}
		}
		if track.Album == "" {
			if dirAlbum := parseDirectoryAlbum(rel); dirAlbum != "" {
				track.Album = dirAlbum
			}
		}
	}
	if len(track.Artists) == 0 {
		track.Artists = []string{"Unknown Artist"}
	}
	if track.Album == "" {
		track.Album = "Unknown Album"
	}
}

// normalizeArtists normalizes each artist string and drops any duplicates or
// empties that result. Returning a deduped slice means artist-keyed indexes
// don't accumulate identical entries.
func normalizeArtists(artists []string) []string {
	if len(artists) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(artists))
	out := make([]string, 0, len(artists))
	for _, a := range artists {
		na := musicmatch.NormalizeForMatch(a)
		if na == "" {
			continue
		}
		if _, dup := seen[na]; dup {
			continue
		}
		seen[na] = struct{}{}
		out = append(out, na)
	}
	return out
}

// parseFilenameArtistTitle attempts to extract (artist, title) from a filename
// using common conventions like "Artist - Title.ext", "01 - Title.ext", or
// "01. Title.ext". If only a title can be derived, artist is returned empty.
func parseFilenameArtistTitle(filename string) (artist, title string) {
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	base = strings.TrimSpace(base)
	if base == "" {
		return "", ""
	}

	if idx := strings.Index(base, " - "); idx >= 0 {
		left := strings.TrimSpace(base[:idx])
		right := strings.TrimSpace(base[idx+3:])
		if right == "" {
			return "", left
		}
		// "01 - Title" or "1 - Title" is a track-number prefix, not an artist.
		if isTrackNumber(left) {
			return "", right
		}
		return left, right
	}
	// "01. Title" / "01 Title"
	if i := strings.IndexAny(base, ". "); i > 0 && isTrackNumber(base[:i]) {
		return "", strings.TrimSpace(strings.TrimLeft(base[i:], ". "))
	}
	return "", base
}

// isTrackNumber reports whether s is a 1- to 3-digit track number.
func isTrackNumber(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 3 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// parseDirectoryArtist returns the artist name implied by the directory
// hierarchy "<root>/Artist/Album (Year)/file.ext". Returns empty when the
// structure is shallower than that.
func parseDirectoryArtist(rel string) string {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) >= 3 {
		return strings.TrimSpace(parts[len(parts)-3])
	}
	return ""
}

// parseDirectoryAlbum returns the album implied by the directory hierarchy,
// stripping a trailing "(YYYY)" year suffix if present.
func parseDirectoryAlbum(rel string) string {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) < 2 {
		return ""
	}
	album := strings.TrimSpace(parts[len(parts)-2])
	album = musicmatch.AlbumYearSuffix.ReplaceAllString(album, "")
	return strings.TrimSpace(album)
}
