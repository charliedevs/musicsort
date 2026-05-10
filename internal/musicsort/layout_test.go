package musicsort

import (
	"reflect"
	"testing"
)

// componentRefs flattens a Layout's components into a (kind, raw, suffix)
// triple-list so test assertions can read declaratively.
type componentRefs []Component

func collect(meta FileMetadata, l *Layout) componentRefs {
	return componentRefs(l.Components(meta))
}

func TestLayoutByName(t *testing.T) {
	for _, name := range LayoutNames() {
		l, err := LayoutByName(name)
		if err != nil {
			t.Fatalf("LayoutByName(%q) returned error: %v", name, err)
		}
		if l.Name != name {
			t.Fatalf("LayoutByName(%q) returned layout with Name=%q", name, l.Name)
		}
	}
	if _, err := LayoutByName("nope"); err == nil {
		t.Fatalf("expected error for unknown layout")
	}
}

func TestLayoutDefault(t *testing.T) {
	if DefaultLayoutName != "artist-album-year" {
		t.Fatalf("DefaultLayoutName = %q, want %q", DefaultLayoutName, "artist-album-year")
	}
	if _, err := LayoutByName(DefaultLayoutName); err != nil {
		t.Fatalf("default layout not registered: %v", err)
	}
}

func TestLayoutComponents(t *testing.T) {
	meta := FileMetadata{
		Artist:      "Mac Miller",
		AlbumArtist: "Mac Miller",
		Album:       "Faces",
		Title:       "Polo Jeans",
		Genre:       "Hip-Hop",
		Year:        " (2014)",
		TrackNumber: 7,
		DiscNumber:  1,
		DiscTotal:   1,
	}
	tests := []struct {
		layout string
		want   []Component
	}{
		{
			layout: "artist-album-year",
			want: []Component{
				{Kind: PartKindArtist, Raw: "Mac Miller"},
				{Kind: PartKindAlbum, Raw: "Faces", Suffix: " (2014)"},
			},
		},
		{
			layout: "artist-album",
			want: []Component{
				{Kind: PartKindArtist, Raw: "Mac Miller"},
				{Kind: PartKindAlbum, Raw: "Faces"},
			},
		},
		{
			layout: "albumartist-album-year",
			want: []Component{
				{Kind: PartKindArtist, Raw: "Mac Miller"},
				{Kind: PartKindAlbum, Raw: "Faces", Suffix: " (2014)"},
			},
		},
		{
			layout: "flat",
			want:   nil,
		},
		{
			layout: "genre-artist-album-year",
			want: []Component{
				{Kind: PartKindGenre, Raw: "Hip-Hop"},
				{Kind: PartKindArtist, Raw: "Mac Miller"},
				{Kind: PartKindAlbum, Raw: "Faces", Suffix: " (2014)"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.layout, func(t *testing.T) {
			l, err := LayoutByName(tt.layout)
			if err != nil {
				t.Fatal(err)
			}
			got := collect(meta, l)
			if !reflect.DeepEqual([]Component(got), tt.want) {
				t.Fatalf("layout %q components = %+v, want %+v", tt.layout, got, tt.want)
			}
		})
	}
}

// TestLayoutComponentFallbacks regression-tests the "Unknown" fallbacks that
// fire when individual tag fields are empty. Without these, the user's
// existing `Mac Miller/Unknown Album` style folders couldn't be reproduced
// for tagless downloads.
func TestLayoutComponentFallbacks(t *testing.T) {
	empty := FileMetadata{}
	l, _ := LayoutByName("artist-album-year")
	got := l.Components(empty)
	want := []Component{
		{Kind: PartKindArtist, Raw: "Unknown Artist"},
		{Kind: PartKindAlbum, Raw: "Unknown Album"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("empty meta produced %+v, want %+v", got, want)
	}

	g, _ := LayoutByName("genre-artist-album-year")
	if comps := g.Components(empty); comps[0].Raw != "Unknown Genre" {
		t.Fatalf("genre fallback = %q, want Unknown Genre", comps[0].Raw)
	}
}

func TestLayoutFilenameTrackNumberPrefix(t *testing.T) {
	meta := FileMetadata{
		Artist:      "Mac Miller",
		Album:       "Faces",
		Title:       "Polo Jeans",
		TrackNumber: 7,
		DiscNumber:  1,
		DiscTotal:   1,
	}
	l, _ := LayoutByName("artist-album-year")

	// Default-on: produces "07 - Mac Miller - Polo Jeans.mp3".
	got := l.Filename(meta, ".mp3", FilenameOpts{IncludeTrackNumber: true})
	want := "07 - Mac Miller - Polo Jeans.mp3"
	if got != want {
		t.Fatalf("with track-number prefix = %q, want %q", got, want)
	}

	// Opt-out: drops the prefix entirely.
	got = l.Filename(meta, ".mp3", FilenameOpts{IncludeTrackNumber: false})
	want = "Mac Miller - Polo Jeans.mp3"
	if got != want {
		t.Fatalf("no track-number prefix = %q, want %q", got, want)
	}

	// TrackNumber == 0: prefix is dropped even when the option is on,
	// because we have no number to render.
	tagless := meta
	tagless.TrackNumber = 0
	got = l.Filename(tagless, ".mp3", FilenameOpts{IncludeTrackNumber: true})
	want = "Mac Miller - Polo Jeans.mp3"
	if got != want {
		t.Fatalf("track-number 0 = %q, want %q", got, want)
	}
}

// TestLayoutFilenameMultiDisc covers the "D-NN - Title" prefix shape used
// when a release is multi-disc. A single-disc release with DiscTotal=1
// (or missing entirely) must NOT include a disc segment.
func TestLayoutFilenameMultiDisc(t *testing.T) {
	meta := FileMetadata{
		Artist:      "Smashing Pumpkins",
		Title:       "Tonight, Tonight",
		TrackNumber: 5,
		DiscNumber:  2,
		DiscTotal:   2,
	}
	l, _ := LayoutByName("artist-album-year")
	got := l.Filename(meta, ".mp3", FilenameOpts{IncludeTrackNumber: true})
	want := "2-05 - Smashing Pumpkins - Tonight, Tonight.mp3"
	if got != want {
		t.Fatalf("multi-disc filename = %q, want %q", got, want)
	}

	// Same release on a single-disc layout should NOT show the disc.
	single := meta
	single.DiscTotal = 1
	got = l.Filename(single, ".mp3", FilenameOpts{IncludeTrackNumber: true})
	want = "05 - Smashing Pumpkins - Tonight, Tonight.mp3"
	if got != want {
		t.Fatalf("single-disc filename = %q, want %q", got, want)
	}
}

func TestLayoutFlatFilename(t *testing.T) {
	meta := FileMetadata{
		Artist:      "Mac Miller",
		Album:       "Faces",
		Title:       "Polo Jeans",
		TrackNumber: 7,
	}
	l, _ := LayoutByName("flat")
	got := l.Filename(meta, ".mp3", FilenameOpts{IncludeTrackNumber: true})
	want := "07 - Mac Miller - Faces - Polo Jeans.mp3"
	if got != want {
		t.Fatalf("flat filename = %q, want %q", got, want)
	}
}

func TestSanitizedComponentName(t *testing.T) {
	c := Component{Kind: PartKindAlbum, Raw: "A/Path:With?Bad*Chars", Suffix: " (2014)"}
	got := SanitizedComponentName(c)
	want := "A_Path_With_Bad_Chars (2014)"
	if got != want {
		t.Fatalf("SanitizedComponentName = %q, want %q", got, want)
	}
}

func TestJoinPath(t *testing.T) {
	comps := []Component{
		{Kind: PartKindArtist, Raw: "Mac Miller"},
		{Kind: PartKindAlbum, Raw: "Faces", Suffix: " (2014)"},
	}
	got := JoinPath(comps, "07 - Mac Miller - Polo Jeans.mp3")
	// filepath.Join uses os-specific separator; check substrings instead
	// so the test passes on both Linux and Windows.
	for _, want := range []string{"Mac Miller", "Faces (2014)", "07 - Mac Miller - Polo Jeans.mp3"} {
		if !contains(got, want) {
			t.Fatalf("JoinPath = %q, missing %q", got, want)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || indexOf(haystack, needle) >= 0)
}

// indexOf is a stripped-down strings.Contains so this test file doesn't
// pull in the strings package just for one assertion.
func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
