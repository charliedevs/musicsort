package musicsort

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// PartKind identifies which piece of metadata a Component derives from. The
// TargetIndex uses these tags to know which directory levels participate in
// the artist / album dedup heuristics: a layout with no PartKindArtist
// component (e.g. `flat`) skips the case-fold dedup pass entirely.
type PartKind int

const (
	PartKindOther PartKind = iota
	// PartKindArtist marks the directory whose value the dedup index should
	// case-fold as the artist. Either musicsort's Artist or AlbumArtist
	// preset choice resolves to this kind.
	PartKindArtist
	// PartKindAlbum marks the directory the dedup index treats as the album
	// for case-fold and edition-variant collapsing.
	PartKindAlbum
	// PartKindGenre marks an outer genre folder (genre-artist-album-year).
	PartKindGenre
)

// Component is one directory level produced by a Layout. The Raw value is
// pre-sanitization so the dedup index can run musicmatch.NormalizeForMatch
// over it; the on-disk folder name is Sanitize(Raw) + Suffix. Suffix is
// concatenated *after* sanitization so layout authors can opt into a
// year-style annotation like " (2014)" without polluting the dedup key.
type Component struct {
	Kind   PartKind
	Raw    string
	Suffix string
}

// FilenameOpts controls optional formatting decisions when rendering the
// terminal filename.
type FilenameOpts struct {
	IncludeTrackNumber bool
}

// Layout maps file metadata to a list of directory components and a final
// filename. Implementations do NOT sanitize their outputs; the caller
// (organizer) handles Sanitize for filesystem safety so the raw values stay
// available for the dedup index.
type Layout struct {
	Name        string
	Description string
	Components  func(meta FileMetadata) []Component
	Filename    func(meta FileMetadata, ext string, opts FilenameOpts) string
}

// AllLayouts holds every preset, registered in the order they should appear
// in --help. The first entry is the default.
var AllLayouts = []*Layout{
	layoutArtistAlbumYear,
	layoutArtistAlbum,
	layoutAlbumArtistAlbumYear,
	layoutFlat,
	layoutGenreArtistAlbumYear,
}

// DefaultLayoutName is the layout selected when --layout isn't provided. We
// keep the existing "Artist/Album (Year)/" hierarchy as the default so an
// unconfigured musicsort run behaves the same as the previous version.
const DefaultLayoutName = "artist-album-year"

// LayoutByName returns the registered layout with the given name, or an
// error listing every valid name.
func LayoutByName(name string) (*Layout, error) {
	for _, l := range AllLayouts {
		if l.Name == name {
			return l, nil
		}
	}
	names := make([]string, 0, len(AllLayouts))
	for _, l := range AllLayouts {
		names = append(names, l.Name)
	}
	return nil, fmt.Errorf("unknown layout %q (valid: %s)", name, strings.Join(names, ", "))
}

// LayoutNames returns the registered preset names in registration order.
// Used by the CLI help to describe --layout.
func LayoutNames() []string {
	names := make([]string, 0, len(AllLayouts))
	for _, l := range AllLayouts {
		names = append(names, l.Name)
	}
	return names
}

// findComponent returns the index of the first component matching kind, or
// -1 if no such component exists. Used by the dedup index when figuring out
// which directory level corresponds to the artist or album for the active
// layout.
func findComponent(comps []Component, kind PartKind) int {
	for i, c := range comps {
		if c.Kind == kind {
			return i
		}
	}
	return -1
}

// renderTitleArtistFilename is the default filename rule for layouts that
// keep an "Artist - Title" basename, optionally prefixed with the track
// number (and disc number for multi-disc releases). It's broken out so
// every nested-folder preset can share the rule; the `flat` preset uses
// renderFlatFilename instead.
func renderTitleArtistFilename(meta FileMetadata, ext string, opts FilenameOpts) string {
	prefix := trackNumberPrefix(meta, opts)
	return prefix + fmt.Sprintf("%s - %s%s", meta.Artist, meta.Title, ext)
}

// renderFlatFilename concatenates artist+album+title for the single-folder
// `flat` layout, where directory structure isn't carrying any of the
// metadata. The track-number prefix still applies so multi-disc releases
// stay sorted alphabetically when listed in a file manager.
func renderFlatFilename(meta FileMetadata, ext string, opts FilenameOpts) string {
	prefix := trackNumberPrefix(meta, opts)
	return prefix + fmt.Sprintf("%s - %s - %s%s", meta.Artist, meta.Album, meta.Title, ext)
}

// trackNumberPrefix returns the leading "NN - " or "D-NN - " segment for a
// filename, or "" if track-number prefixing is disabled or the tag value
// isn't usable. We pad single-digit numbers to two digits so file managers
// sort tracks correctly without needing track-number-aware sorting.
func trackNumberPrefix(meta FileMetadata, opts FilenameOpts) string {
	if !opts.IncludeTrackNumber || meta.TrackNumber <= 0 {
		return ""
	}
	if meta.DiscTotal > 1 && meta.DiscNumber > 0 {
		return fmt.Sprintf("%d-%02d - ", meta.DiscNumber, meta.TrackNumber)
	}
	return fmt.Sprintf("%02d - ", meta.TrackNumber)
}

// JoinPath assembles the relative path for a rendered file: it sanitizes
// each component, appends each Suffix verbatim (so " (2014)" survives), and
// joins them with the platform separator. The resulting path is suitable
// for filepath.Join with the target directory.
func JoinPath(comps []Component, filename string) string {
	parts := make([]string, 0, len(comps)+1)
	for _, c := range comps {
		parts = append(parts, Sanitize(c.Raw)+c.Suffix)
	}
	parts = append(parts, filename)
	return filepath.Join(parts...)
}

// SanitizedComponentName mirrors what JoinPath does for a single component:
// the on-disk folder name for a Component is Sanitize(Raw)+Suffix. Exposed
// so the dedup index can compare the sanitized form of an incoming file
// against the literal folder names it scanned off disk.
func SanitizedComponentName(c Component) string {
	return Sanitize(c.Raw) + c.Suffix
}

// nonEmptyOrDefault returns value if non-empty, fallback otherwise. Used by
// layouts to substitute "Unknown Artist" / "Unknown Album" / "Unknown Genre"
// when the corresponding tag is missing.
func nonEmptyOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// --- Preset definitions ---

// layoutArtistAlbumYear is the original musicsort hierarchy preserved as
// the default: "Artist/Album (Year)/Artist - Title.ext".
var layoutArtistAlbumYear = &Layout{
	Name:        "artist-album-year",
	Description: "Artist/Album (Year)/Artist - Title.ext (default)",
	Components: func(meta FileMetadata) []Component {
		return []Component{
			{Kind: PartKindArtist, Raw: nonEmptyOrDefault(meta.Artist, "Unknown Artist")},
			{Kind: PartKindAlbum, Raw: nonEmptyOrDefault(meta.Album, "Unknown Album"), Suffix: meta.Year},
		}
	},
	Filename: renderTitleArtistFilename,
}

// layoutArtistAlbum drops the year suffix from the album folder for users
// who prefer a cleaner tree. The dedup index treats both with-year and
// without-year folders identically anyway (the year suffix is stripped
// before normalization), so switching back and forth doesn't fragment a
// library.
var layoutArtistAlbum = &Layout{
	Name:        "artist-album",
	Description: "Artist/Album/Artist - Title.ext (no year suffix)",
	Components: func(meta FileMetadata) []Component {
		return []Component{
			{Kind: PartKindArtist, Raw: nonEmptyOrDefault(meta.Artist, "Unknown Artist")},
			{Kind: PartKindAlbum, Raw: nonEmptyOrDefault(meta.Album, "Unknown Album")},
		}
	},
	Filename: renderTitleArtistFilename,
}

// layoutAlbumArtistAlbumYear keys the artist directory off AlbumArtist
// rather than Artist. This is the preferred layout for libraries with lots
// of compilations or featured-artist tracks: a Beyoncé album's "Telephone
// (feat. Lady Gaga)" sits next to its sibling tracks instead of getting
// punted into a "Beyoncé;Lady Gaga" folder.
var layoutAlbumArtistAlbumYear = &Layout{
	Name:        "albumartist-album-year",
	Description: "AlbumArtist/Album (Year)/Artist - Title.ext (compilation-friendly)",
	Components: func(meta FileMetadata) []Component {
		// AlbumArtist falls back to Artist when missing so a freshly-tagged
		// solo release doesn't end up in "Unknown Artist" just because the
		// tagger only filled in TPE1.
		artist := meta.AlbumArtist
		if strings.TrimSpace(artist) == "" {
			artist = meta.Artist
		}
		return []Component{
			{Kind: PartKindArtist, Raw: nonEmptyOrDefault(artist, "Unknown Artist")},
			{Kind: PartKindAlbum, Raw: nonEmptyOrDefault(meta.Album, "Unknown Album"), Suffix: meta.Year},
		}
	},
	Filename: renderTitleArtistFilename,
}

// layoutFlat puts every track in a single folder. Useful for portable
// players that don't render directory trees gracefully, or for users who
// prefer to drive organization entirely off filename. Returning a nil
// Components slice lets the organizer skip directory creation entirely
// and write straight into the target dir.
var layoutFlat = &Layout{
	Name:        "flat",
	Description: "Single directory; filename is Artist - Album - Title.ext",
	Components: func(meta FileMetadata) []Component {
		return nil
	},
	Filename: renderFlatFilename,
}

// layoutGenreArtistAlbumYear adds an outer genre folder for users who
// browse by genre on a portable device. Genre is read from the audio tag
// (FileMetadata.Genre) and falls back to "Unknown Genre" when absent.
var layoutGenreArtistAlbumYear = &Layout{
	Name:        "genre-artist-album-year",
	Description: "Genre/Artist/Album (Year)/Artist - Title.ext",
	Components: func(meta FileMetadata) []Component {
		return []Component{
			{Kind: PartKindGenre, Raw: nonEmptyOrDefault(meta.Genre, "Unknown Genre")},
			{Kind: PartKindArtist, Raw: nonEmptyOrDefault(meta.Artist, "Unknown Artist")},
			{Kind: PartKindAlbum, Raw: nonEmptyOrDefault(meta.Album, "Unknown Album"), Suffix: meta.Year},
		}
	},
	Filename: renderTitleArtistFilename,
}

// LayoutHelpText renders the layout list as a human-readable bullet list,
// suitable for embedding in --help output. Sorted by registration order so
// the default is on top.
func LayoutHelpText() string {
	var b strings.Builder
	b.WriteString("Available layouts:\n")
	names := LayoutNames()
	sort.SliceStable(names, func(i, j int) bool {
		return i < j
	})
	for _, l := range AllLayouts {
		fmt.Fprintf(&b, "  %s — %s\n", l.Name, l.Description)
	}
	return b.String()
}
