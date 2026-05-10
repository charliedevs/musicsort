package spotifym3u

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"musicsort/internal/musicmatch"
)

func TestParseSpotifyCSV(t *testing.T) {
	// Legacy short-form columns we still support.
	const csvContent = `URI,Track Name,Album,Artist,Release,Duration (ms)
spotify:track:1,Test Song,Test Album,Test Artist,2024,180000
spotify:track:2,Collab Song,Collab Album,Artist A; Artist B,2024,200000
`
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "playlist.csv")
	if err := os.WriteFile(path, []byte(csvContent), 0o600); err != nil {
		t.Fatal(err)
	}

	entries, err := ParseSpotifyCSV(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].TrackName != "Test Song" || len(entries[0].Artists) != 1 || entries[0].Artists[0] != "Test Artist" {
		t.Fatalf("unexpected entry 0: %+v", entries[0])
	}
	if entries[1].TrackName != "Collab Song" || !reflect.DeepEqual(entries[1].Artists, []string{"Artist A", "Artist B"}) {
		t.Fatalf("unexpected entry 1: %+v", entries[1])
	}
}

// TestParseExportifyCSV exercises the actual column header that Exportify
// emits. Without column-name aliasing, every Artist/Album/URI/Release field
// would silently come back empty and matching would collapse for every user.
func TestParseExportifyCSV(t *testing.T) {
	const csvContent = `Track URI,Track Name,Album Name,Artist Name(s),Release Date,Duration (ms),Popularity,Explicit
spotify:track:abc,"Halo","I Am... Sasha Fierce","Beyoncé",2008-11-14,261600,82,false
spotify:track:def,"King Gizzard Track","Sketches of Brunswick East","King Gizzard & The Lizard Wizard;Mild High Club",2017-08-18,196026,52,false
spotify:track:ghi,"Our House","Deja Vu","Crosby, Stills, Nash & Young",1970-03-11,179760,73,false
spotify:track:jkl,"Kellett St.","Cold Radish Coma","30/70",2015-12-18,79152,5,false
`
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "exportify.csv")
	if err := os.WriteFile(path, []byte(csvContent), 0o600); err != nil {
		t.Fatal(err)
	}

	entries, err := ParseSpotifyCSV(path)
	if err != nil {
		t.Fatalf("ParseSpotifyCSV: %v", err)
	}
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(entries))
	}

	// Row 0: single artist, columns must be wired through the long-form aliases.
	e := entries[0]
	if e.URI != "spotify:track:abc" {
		t.Errorf("URI = %q, want %q", e.URI, "spotify:track:abc")
	}
	if e.TrackName != "Halo" {
		t.Errorf("TrackName = %q, want %q", e.TrackName, "Halo")
	}
	if e.Album != "I Am... Sasha Fierce" {
		t.Errorf("Album = %q, want %q", e.Album, "I Am... Sasha Fierce")
	}
	if !reflect.DeepEqual(e.Artists, []string{"Beyoncé"}) {
		t.Errorf("Artists = %v, want [Beyoncé]", e.Artists)
	}
	if e.Release != "2008-11-14" {
		t.Errorf("Release = %q, want %q", e.Release, "2008-11-14")
	}
	if e.Duration != 261 {
		t.Errorf("Duration = %d seconds, want 261", e.Duration)
	}

	// Row 1: collaboration via `;`, but the leading band's `&` must stay intact.
	if !reflect.DeepEqual(entries[1].Artists, []string{"King Gizzard & The Lizard Wizard", "Mild High Club"}) {
		t.Errorf("collab artists = %v, want [King Gizzard & The Lizard Wizard, Mild High Club]", entries[1].Artists)
	}

	// Row 2: single artist whose name contains commas and `&` must NOT be split.
	if !reflect.DeepEqual(entries[2].Artists, []string{"Crosby, Stills, Nash & Young"}) {
		t.Errorf("CSNY artists = %v, want [Crosby, Stills, Nash & Young]", entries[2].Artists)
	}

	// Row 3: artist name contains `/` and must stay a single entry.
	if !reflect.DeepEqual(entries[3].Artists, []string{"30/70"}) {
		t.Errorf("30/70 artists = %v, want [30/70]", entries[3].Artists)
	}
}

// makeIndex builds a TrackIndex populated from a slice of localTrack values.
// Delegates to (*TrackIndex).addTrack so unit tests exercise the same
// indexing rules production uses.
func makeIndex(tracks []*localTrack) *TrackIndex {
	idx := newTrackIndex("")
	for _, t := range tracks {
		idx.addTrack(t)
	}
	return idx
}

func makeLocalTrack(path, title, album string, artists ...string) *localTrack {
	t := &localTrack{
		Path:         path,
		Title:        title,
		Album:        album,
		Artists:      artists,
		Filename:     filepath.Base(path),
		NormFilename: musicmatch.NormalizeForMatch(filepath.Base(path)),
		NormTitle:    musicmatch.NormalizeForMatch(title),
		NormAlbum:    musicmatch.NormalizeForMatch(album),
	}
	t.NormArtists = normalizeArtists(artists)
	t.TitleTokens = musicmatch.TokenSet(t.NormTitle)
	return t
}

func TestFindBestMatchScoring(t *testing.T) {
	exact := makeLocalTrack("a/exact.mp3", "Song", "Album", "Artist")
	titleArtistOnly := makeLocalTrack("b/ta.mp3", "Song", "Different Album", "Artist")
	titleAlbumOnly := makeLocalTrack("c/tab.mp3", "Song", "Album", "Different Artist")
	titleHash := makeLocalTrack("d/title.mp3", "Song", "Other Album", "Some Other")
	titleSubstring := makeLocalTrack("e/sub.mp3", "Song Extended", "Whatever", "Whoever")
	tokenish := makeLocalTrack("f/token.mp3", "Halo Live", "Album", "Artist")
	artistOnlyTrack := makeLocalTrack("g/artist.mp3", "Totally Unrelated", "Whatever", "Artist")
	filenameTrack := makeLocalTrack("h/song-thing.mp3", "", "", "")
	filenameTrack.Title = "song-thing"
	filenameTrack.NormTitle = musicmatch.NormalizeForMatch("song-thing")
	filenameTrack.NormFilename = musicmatch.NormalizeForMatch("song-thing.mp3")
	filenameTrack.TitleTokens = musicmatch.TokenSet(filenameTrack.NormTitle)

	tests := []struct {
		name          string
		index         *TrackIndex
		entry         PlaylistEntry
		wantPath      string
		wantReasonHas string
	}{
		{
			name:          "exact title+artist+album wins over weaker matches",
			index:         makeIndex([]*localTrack{exact, titleArtistOnly, titleAlbumOnly, titleHash}),
			entry:         PlaylistEntry{TrackName: "Song", Album: "Album", Artists: []string{"Artist"}},
			wantPath:      "a/exact.mp3",
			wantReasonHas: "title+artist+album",
		},
		{
			name:          "title+artist beats title+album when album differs",
			index:         makeIndex([]*localTrack{titleArtistOnly, titleAlbumOnly}),
			entry:         PlaylistEntry{TrackName: "Song", Album: "Album", Artists: []string{"Artist"}},
			wantPath:      "b/ta.mp3",
			wantReasonHas: "title+artist",
		},
		{
			name:          "title+album when artist differs",
			index:         makeIndex([]*localTrack{titleAlbumOnly}),
			entry:         PlaylistEntry{TrackName: "Song", Album: "Album", Artists: []string{"Artist"}},
			wantPath:      "c/tab.mp3",
			wantReasonHas: "title+album",
		},
		{
			name:          "title-only hash falls back when no artist/album info",
			index:         makeIndex([]*localTrack{titleHash}),
			entry:         PlaylistEntry{TrackName: "Song"},
			wantPath:      "d/title.mp3",
			wantReasonHas: "title",
		},
		{
			name:          "substring title fallback",
			index:         makeIndex([]*localTrack{titleSubstring}),
			entry:         PlaylistEntry{TrackName: "Song"},
			wantPath:      "e/sub.mp3",
			wantReasonHas: "title",
		},
		{
			name:          "token-similarity catches reordering",
			index:         makeIndex([]*localTrack{tokenish}),
			entry:         PlaylistEntry{TrackName: "Live Halo", Artists: []string{"Artist"}, Album: "Album"},
			wantPath:      "f/token.mp3",
			wantReasonHas: "token-similarity",
		},
		{
			// Artist-only now requires at least one shared title token so
			// distinct missing songs by the same artist no longer all fan
			// out onto the same alphabetically-first file.
			name:          "artist-only fallback fires when at least one title token overlaps",
			index:         makeIndex([]*localTrack{artistOnlyTrack}),
			entry:         PlaylistEntry{TrackName: "Totally Different Track", Artists: []string{"Artist"}},
			wantPath:      "g/artist.mp3",
			wantReasonHas: "artist",
		},
		{
			name:          "filename substring catches metadata-less files",
			index:         makeIndex([]*localTrack{filenameTrack}),
			entry:         PlaylistEntry{TrackName: "song"},
			wantPath:      "h/song-thing.mp3",
			wantReasonHas: "title",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate, reason := tt.index.findBestMatch(tt.entry)
			if candidate == nil {
				t.Fatalf("expected match, got nil; reason=%q", reason)
			}
			if candidate.Path != tt.wantPath {
				t.Fatalf("matched %q, want %q (reason=%q)", candidate.Path, tt.wantPath, reason)
			}
			if !strings.Contains(reason, tt.wantReasonHas) {
				t.Fatalf("reason %q does not contain %q", reason, tt.wantReasonHas)
			}
		})
	}
}

func TestFindBestMatchTieBreakIsDeterministic(t *testing.T) {
	a := makeLocalTrack("z/song.mp3", "Song", "Album", "Artist")
	b := makeLocalTrack("a/song.mp3", "Song", "Album", "Artist")
	idx := makeIndex([]*localTrack{a, b})
	candidate, _ := idx.findBestMatch(PlaylistEntry{TrackName: "Song", Album: "Album", Artists: []string{"Artist"}})
	if candidate == nil || candidate.Path != "a/song.mp3" {
		t.Fatalf("expected lexicographically smallest path to win tie, got %v", candidate)
	}
}

func TestBuildIndexAndMatch(t *testing.T) {
	track := makeLocalTrack("artist/album/song.mp3", "Song", "Album Name", "Some Artist")
	idx := makeIndex([]*localTrack{track})

	entries := []PlaylistEntry{{TrackName: "Song", Artists: []string{"Some Artist"}, Album: "Album Name", Duration: 213}}
	result := idx.MatchPlaylist(entries, false)
	if result.Matched != 1 {
		t.Fatalf("expected 1 match, got %d", result.Matched)
	}
	if result.Matches[0].Candidate.Path != track.Path {
		t.Fatalf("matched wrong track, got %q", result.Matches[0].Candidate.Path)
	}
}

// TestBuildIndexPopulatesAllTracks regression-tests the bug that caused this
// whole effort: BuildIndex must populate index.allTracks (and titleOnly) so
// the fuzzy fallbacks in findBestMatch have something to iterate. We use a
// .mp3 file with no real audio data; tag.ReadFrom will fail and buildTrack
// falls back to filename-derived fields, which is exactly the case where the
// fuzzy fallbacks matter most.
func TestBuildIndexPopulatesAllTracks(t *testing.T) {
	tmpDir := t.TempDir()
	files := []string{"Artist Name - Song One.mp3", "Artist Name - Song Two.flac"}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte("not real audio"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	idx, err := BuildIndex(tmpDir, false, false)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	if len(idx.allTracks) != len(files) {
		t.Fatalf("allTracks has %d entries, want %d", len(idx.allTracks), len(files))
	}
	if len(idx.titleOnly) == 0 {
		t.Fatal("titleOnly map should be populated")
	}
	if len(idx.exactMatch) == 0 {
		t.Fatal("exactMatch map should be populated")
	}

	// Each indexed track's filename should be reachable through allTracks.
	paths := make([]string, 0, len(idx.allTracks))
	for _, tr := range idx.allTracks {
		paths = append(paths, tr.Filename)
	}
	sort.Strings(paths)
	want := []string{"Artist Name - Song One.mp3", "Artist Name - Song Two.flac"}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("indexed filenames = %v, want %v", paths, want)
	}
}

// TestBuildIndexEnablesFuzzyMatch ensures that an entry whose normalized title
// only appears as a substring of a local file's filename still gets matched.
// This is the failure mode users were hitting after the regression.
func TestBuildIndexEnablesFuzzyMatch(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "Beyonce - Halo.mp3"), []byte("not real audio"), 0o600); err != nil {
		t.Fatal(err)
	}

	idx, err := BuildIndex(tmpDir, false, false)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

	entry := PlaylistEntry{TrackName: "Halo", Album: "I Am... Sasha Fierce", Artists: []string{"Beyoncé"}}
	candidate, reason := idx.findBestMatch(entry)
	if candidate == nil {
		t.Fatalf("expected fuzzy match for %q, got nil (reason=%q)", entry.TrackName, reason)
	}
	if !strings.Contains(candidate.Filename, "Halo") {
		t.Fatalf("matched unexpected filename: %q (reason=%q)", candidate.Filename, reason)
	}
}

func TestParseFilenameArtistTitle(t *testing.T) {
	tests := []struct {
		input      string
		wantArtist string
		wantTitle  string
	}{
		{"Beyoncé - Halo.mp3", "Beyoncé", "Halo"},
		{"Daft Punk - Get Lucky.flac", "Daft Punk", "Get Lucky"},
		{"01 - Halo.mp3", "", "Halo"},
		{"01. Halo.mp3", "", "Halo"},
		{"Halo.mp3", "", "Halo"},
		{"  Spaced - Out  .mp3", "Spaced", "Out"},
		{"", "", ""},
	}
	for _, tt := range tests {
		gotArtist, gotTitle := parseFilenameArtistTitle(tt.input)
		if gotArtist != tt.wantArtist || gotTitle != tt.wantTitle {
			t.Errorf("parseFilenameArtistTitle(%q) = (%q, %q), want (%q, %q)",
				tt.input, gotArtist, gotTitle, tt.wantArtist, tt.wantTitle)
		}
	}
}

func TestParseDirectoryArtistAndAlbum(t *testing.T) {
	tests := []struct {
		rel        string
		wantArtist string
		wantAlbum  string
	}{
		{"Beyoncé/I Am... Sasha Fierce (2008)/Beyoncé - Halo.mp3", "Beyoncé", "I Am... Sasha Fierce"},
		{"Daft Punk/Random Access Memories/01 - Get Lucky.mp3", "Daft Punk", "Random Access Memories"},
		{"Album (2024)/song.mp3", "", "Album"},
		{"song.mp3", "", ""},
	}
	for _, tt := range tests {
		gotArtist := parseDirectoryArtist(tt.rel)
		gotAlbum := parseDirectoryAlbum(tt.rel)
		if gotArtist != tt.wantArtist || gotAlbum != tt.wantAlbum {
			t.Errorf("parseDirectory(%q) = (%q, %q), want (%q, %q)",
				tt.rel, gotArtist, gotAlbum, tt.wantArtist, tt.wantAlbum)
		}
	}
}

func TestBuildIndexUsesFilenameAndDirFallbacks(t *testing.T) {
	tmpDir := t.TempDir()
	dir := filepath.Join(tmpDir, "Beyoncé", "I Am... Sasha Fierce (2008)")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Beyoncé - Single Ladies.mp3"), []byte("not real audio"), 0o600); err != nil {
		t.Fatal(err)
	}

	idx, err := BuildIndex(tmpDir, true, false)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	if len(idx.allTracks) != 1 {
		t.Fatalf("expected 1 indexed track, got %d", len(idx.allTracks))
	}
	tr := idx.allTracks[0]
	if tr.Title != "Single Ladies" {
		t.Errorf("Title = %q, want %q", tr.Title, "Single Ladies")
	}
	if len(tr.Artists) == 0 || tr.Artists[0] != "Beyoncé" {
		t.Errorf("Artists = %v, want [Beyoncé]", tr.Artists)
	}
	if tr.Album != "I Am... Sasha Fierce" {
		t.Errorf("Album = %q, want %q", tr.Album, "I Am... Sasha Fierce")
	}

	entry := PlaylistEntry{TrackName: "Single Ladies (Put a Ring on It)", Album: "I Am... Sasha Fierce", Artists: []string{"Beyoncé"}}
	candidate, reason := idx.findBestMatch(entry)
	if candidate == nil {
		t.Fatalf("expected match via filename/directory fallback, got nil (reason=%q)", reason)
	}
}

func TestClosestCandidatesProducesNearMisses(t *testing.T) {
	a := makeLocalTrack("a/halo.mp3", "Halo", "Album", "Beyoncé")
	b := makeLocalTrack("b/single ladies.mp3", "Single Ladies", "Album", "Beyoncé")
	idx := makeIndex([]*localTrack{a, b})

	entry := PlaylistEntry{TrackName: "Halo Live", Artists: []string{"Beyoncé"}}
	got := idx.closestCandidates(entry, 2)
	if len(got) == 0 {
		t.Fatal("expected at least one near-miss candidate, got none")
	}
	if got[0].Track.Path != "a/halo.mp3" {
		t.Fatalf("expected halo to be closest, got %q", got[0].Track.Path)
	}
}

// TestTitleSubstringRequiresArtistMatchForCrossArtist regression-tests the
// Marvin Gaye -> Jim Guthrie bug. The local file's title ("Trouble") is a
// word-aligned substring of the CSV title ("Main Theme From Trouble Man - 2"),
// but the artists are unrelated, so the substring fallback must not fire.
func TestTitleSubstringRequiresArtistMatchForCrossArtist(t *testing.T) {
	jimGuthrieTrouble := makeLocalTrack(
		"Jim Guthrie/Morning Noon Night/Jim Guthrie - Trouble.mp3",
		"Trouble", "Morning Noon Night", "Jim Guthrie",
	)
	idx := makeIndex([]*localTrack{jimGuthrieTrouble})

	entry := PlaylistEntry{
		TrackName: "Main Theme From Trouble Man - 2",
		Album:     "Trouble Man",
		Artists:   []string{"Marvin Gaye"},
	}
	candidate, reason := idx.findBestMatch(entry)
	if candidate != nil {
		t.Fatalf("expected no match (cross-artist title-substring should not fire), got %q (reason=%q)", candidate.Path, reason)
	}
}

// TestShortLocalTitleDoesNotMatchUnrelatedCSVRow regression-tests the "As"
// fanout: Stevie Wonder's "As" used to match "Ask Me No Questions" and
// "Asshtonpark" because the 2-char NormTitle character-substring-matched
// into longer unrelated CSV titles.
func TestShortLocalTitleDoesNotMatchUnrelatedCSVRow(t *testing.T) {
	stevieAs := makeLocalTrack(
		"Stevie Wonder/Songs In The Key Of Life/Stevie Wonder - As.mp3",
		"As", "Songs In The Key Of Life", "Stevie Wonder",
	)
	idx := makeIndex([]*localTrack{stevieAs})

	for _, entry := range []PlaylistEntry{
		{TrackName: "Ask Me No Questions - Single Version", Album: "Indianola Mississippi Seeds", Artists: []string{"B.B. King", "Leon Russell"}},
		{TrackName: "Asshtonpark", Album: "Rides Again", Artists: []string{"James Gang"}},
	} {
		candidate, reason := idx.findBestMatch(entry)
		if candidate != nil {
			t.Fatalf("expected no match for %q -> %q, but got %q (reason=%q)",
				entry.TrackName, stevieAs.Path, candidate.Path, reason)
		}
	}
}

// TestArtistOnlyFallbackDoesNotFanOutAcrossUnrelatedTitles regression-tests
// the Robin Trower / Susan Tedeschi / Kali Uchis / Pink Floyd duplicate-
// matches issue. Five CSV rows with no token overlap to the single available
// local track must NOT all collapse onto that one track via artistOnly.
func TestArtistOnlyFallbackDoesNotFanOutAcrossUnrelatedTitles(t *testing.T) {
	fallingStar := makeLocalTrack(
		"Robin Trower/1977-10-16 - Music Hall, Boston/Robin Trower - Falling Star.mp3",
		"Falling Star", "1977-10-16 - Music Hall, Boston", "Robin Trower",
	)
	idx := makeIndex([]*localTrack{fallingStar})

	missing := []PlaylistEntry{
		{TrackName: "Same Rain Falls", Album: "Long Misty Days", Artists: []string{"Robin Trower"}},
		{TrackName: "Lady Love - 2007 Remaster", Album: "Bridge Of Sighs", Artists: []string{"Robin Trower"}},
		{TrackName: "Caroline", Album: "Passion", Artists: []string{"Robin Trower"}},
		{TrackName: "Captain Midnight", Album: "Back It Up", Artists: []string{"Robin Trower"}},
	}
	for _, entry := range missing {
		candidate, reason := idx.findBestMatch(entry)
		if candidate != nil {
			t.Fatalf("expected miss for %q (no token overlap with %q) but got match %q (reason=%q)",
				entry.TrackName, fallingStar.Title, candidate.Path, reason)
		}
	}

	// The CSV row that DOES correspond to the local file must still match.
	candidate, reason := idx.findBestMatch(PlaylistEntry{
		TrackName: "Falling Star",
		Album:     "In City Dreams",
		Artists:   []string{"Robin Trower"},
	})
	if candidate == nil {
		t.Fatalf("expected real Falling Star CSV row to match, got nil (reason=%q)", reason)
	}
	if candidate.Path != fallingStar.Path {
		t.Fatalf("matched %q, want %q", candidate.Path, fallingStar.Path)
	}
}

// TestTitleOnlyHashRequiresArtistMatchForCrossArtist regression-tests the
// "Dragonfly", "Time", "Intro", "All I Need", "Emily" family of false
// positives. The CSV row carries artist info, but the user's only file
// with that exact title is by a *different* artist (and is a different
// song entirely). titleOnly hash used to fire unconditionally at score 40;
// now it must skip when no candidate shares an artist with the entry.
func TestTitleOnlyHashRequiresArtistMatchForCrossArtist(t *testing.T) {
	cases := []struct {
		name  string
		track *localTrack
		entry PlaylistEntry
	}{
		{
			name:  "Dragonfly: Mort Garson asks, Dana and Alden file should not match",
			track: makeLocalTrack("Dana and Alden/Quiet Music/Dana and Alden - Dragonfly.mp3", "Dragonfly", "Quiet Music for Young People", "Dana and Alden"),
			entry: PlaylistEntry{TrackName: "Dragonfly", Album: "Dragonfly", Artists: []string{"Mort Garson"}},
		},
		{
			name:  "Time: Ty Segall asks, Pink Floyd file should not match",
			track: makeLocalTrack("Pink Floyd/The Dark Side Of The Moon/Pink Floyd - Time.flac", "Time", "The Dark Side Of The Moon", "Pink Floyd"),
			entry: PlaylistEntry{TrackName: "Time", Album: "Hair", Artists: []string{"Ty Segall", "White Fence"}},
		},
		{
			name:  "Intro: NANORAY asks, Jim Guthrie file should not match",
			track: makeLocalTrack("Jim Guthrie/A Thousand Songs/Jim Guthrie - Intro.mp3", "Intro", "A Thousand Songs", "Jim Guthrie"),
			entry: PlaylistEntry{TrackName: "Intro", Album: "TILT", Artists: []string{"NANORAY"}},
		},
		{
			name:  "Emily: Glossy asks, Bill Evans file should not match",
			track: makeLocalTrack("Bill Evans/Final Sessions/Bill Evans - Emily.mp3", "Emily", "Final Sessions", "Bill Evans"),
			entry: PlaylistEntry{TrackName: "Emily", Album: "I'm So Sorry", Artists: []string{"Glossy"}},
		},
		{
			name:  "All I Need: The Frights asks, Radiohead file should not match",
			track: makeLocalTrack("Radiohead/In Rainbows/Radiohead - All I Need.opus", "All I Need", "In Rainbows", "Radiohead"),
			entry: PlaylistEntry{TrackName: "All I Need", Album: "You Are Going to Hate This", Artists: []string{"The Frights"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			idx := makeIndex([]*localTrack{tc.track})
			candidate, reason := idx.findBestMatch(tc.entry)
			if candidate != nil {
				t.Fatalf("expected miss (cross-artist titleOnly should be gated) but got %q (reason=%q)", candidate.Path, reason)
			}
		})
	}
}

// TestTitleOnlyHashStillFiresForSameArtistAndSameTitle confirms the legitimate
// case still works: same title, same artist, different album → titleOnly hash
// should still match at scoreTitleHash.
func TestTitleOnlyHashStillFiresForSameArtistAndSameTitle(t *testing.T) {
	track := makeLocalTrack("Beyonce/I Am... Sasha Fierce/Beyonce - Halo.mp3", "Halo", "I Am... Sasha Fierce", "Beyonce")
	idx := makeIndex([]*localTrack{track})

	// Different album triggers neither title+album nor title+artist+album,
	// so titleOnly is the only path that should fire.
	entry := PlaylistEntry{TrackName: "Halo", Album: "Some Compilation", Artists: []string{"Beyoncé"}}
	candidate, reason := idx.findBestMatch(entry)
	if candidate == nil {
		t.Fatalf("expected match via titleOnly with shared artist, got nil (reason=%q)", reason)
	}
	if candidate.Path != track.Path {
		t.Fatalf("matched %q, want %q (reason=%q)", candidate.Path, track.Path, reason)
	}
}

// TestTitleOnlyHashFiresWhenEntryHasNoArtist confirms the artist-info-less
// case still works (e.g. CSV rows that for some reason lack the artist column).
// Without artist info we have nothing to gate on, so titleOnly should match
// any same-titled file.
func TestTitleOnlyHashFiresWhenEntryHasNoArtist(t *testing.T) {
	track := makeLocalTrack("Pink Floyd/Dark Side/Pink Floyd - Time.flac", "Time", "Dark Side", "Pink Floyd")
	idx := makeIndex([]*localTrack{track})

	entry := PlaylistEntry{TrackName: "Time"}
	candidate, reason := idx.findBestMatch(entry)
	if candidate == nil {
		t.Fatalf("expected match via titleOnly with no entry artist, got nil (reason=%q)", reason)
	}
	if candidate.Path != track.Path {
		t.Fatalf("matched %q, want %q", candidate.Path, track.Path)
	}
}

// TestFilenameFallbackRequiresArtistMatchForCrossArtist regression-tests the
// pattern where a CSV title (e.g. "Witch", "Ghost", "Changing") happens to
// be a word-aligned substring of a totally unrelated artist's filename.
// Without a shared-artist gate the filename fallback (score 10) lets these
// false positives slip through whenever no other path fires.
func TestFilenameFallbackRequiresArtistMatchForCrossArtist(t *testing.T) {
	cases := []struct {
		name  string
		track *localTrack
		entry PlaylistEntry
	}{
		{
			name:  "Witch (Alex G) should not match Nardo Wick - Wicked Witch.mp3",
			track: makeLocalTrack("Nardo Wick/Who is Nardo Wick/Nardo Wick - Wicked Witch.mp3", "Wicked Witch", "Who is Nardo Wick", "Nardo Wick"),
			entry: PlaylistEntry{TrackName: "Witch", Album: "Rocket", Artists: []string{"Alex G"}},
		},
		{
			name:  "Ghost (Machine Girl) should not match Fever The Ghost - Sun Moth.opus",
			track: makeLocalTrack("Fever The Ghost/Zirconium Meconium/Fever The Ghost - Sun Moth.opus", "Sun Moth", "Zirconium Meconium", "Fever The Ghost"),
			entry: PlaylistEntry{TrackName: "Ghost", Album: "Wlfgrl", Artists: []string{"Machine Girl"}},
		},
		{
			name:  "Changing (Witch) should not match Boards Of Canada - Constants Are Changing.mp3",
			track: makeLocalTrack("Boards Of Canada/The Campfire Headphase/Boards Of Canada - Constants Are Changing.mp3", "Constants Are Changing", "The Campfire Headphase", "Boards Of Canada"),
			entry: PlaylistEntry{TrackName: "Changing", Album: "Witch", Artists: []string{"Witch"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			idx := makeIndex([]*localTrack{tc.track})
			candidate, reason := idx.findBestMatch(tc.entry)
			if candidate != nil {
				t.Fatalf("expected miss (cross-artist filename should be gated) but got %q (reason=%q)", candidate.Path, reason)
			}
		})
	}
}

// TestFilenameFallbackFiresWhenEntryHasNoArtist confirms that the filename
// fallback's gate doesn't kill the metadata-less-and-artistless case the
// fallback is meant to catch in the first place.
func TestFilenameFallbackFiresWhenEntryHasNoArtist(t *testing.T) {
	track := makeLocalTrack("h/song-thing.mp3", "", "", "")
	track.Title = "song-thing"
	track.NormTitle = musicmatch.NormalizeForMatch("song-thing")
	track.NormFilename = musicmatch.NormalizeForMatch("song-thing.mp3")
	track.TitleTokens = musicmatch.TokenSet(track.NormTitle)
	idx := makeIndex([]*localTrack{track})

	entry := PlaylistEntry{TrackName: "song-thing"}
	candidate, reason := idx.findBestMatch(entry)
	if candidate == nil {
		t.Fatalf("expected match via filename fallback with no entry artist, got nil (reason=%q)", reason)
	}
	if candidate.Path != track.Path {
		t.Fatalf("matched %q, want %q (reason=%q)", candidate.Path, track.Path, reason)
	}
}

// TestArtistFuzzyMatchInTitleOnlyHash exercises the end-to-end matching
// behaviour for the regression cases we want fuzzy artist matching to
// recover. These all start as titleOnly hash hits where the file's
// artist tag is a strict superset of the entry's artist (or vice versa);
// the fuzzy path must let them through without re-introducing the
// unrelated-artist false positives the strict gate is there to block.
func TestArtistFuzzyMatchInTitleOnlyHash(t *testing.T) {
	cases := []struct {
		name      string
		track     *localTrack
		entry     PlaylistEntry
		wantMatch bool
	}{
		{
			name:      "Wings entry recovers Paul McCartney And Wings file via fuzzy",
			track:     makeLocalTrack("Paul McCartney And Wings/Back To The Egg/Paul McCartney And Wings - Reception.flac", "Reception", "Back To The Egg", "Paul McCartney And Wings"),
			entry:     PlaylistEntry{TrackName: "Reception", Album: "Back To The Egg", Artists: []string{"Wings"}},
			wantMatch: true,
		},
		{
			name:      "Emancipator + Cloudchord entry recovers combined-artist file",
			track:     makeLocalTrack("Emancipator & Cloudchord/Citrus Fever Dream/Emancipator & Cloudchord - Thumper.opus", "Thumper", "Citrus Fever Dream", "Emancipator & Cloudchord"),
			entry:     PlaylistEntry{TrackName: "Thumper", Album: "Thumper", Artists: []string{"Emancipator", "Cloudchord"}},
			wantMatch: true,
		},
		{
			name:      "Alice Coltrane entry recovers Various Artists comp file with artist sub-tag",
			track:     makeLocalTrack("Various Artists/100 Greatest/Various Artists - Journey In Satchidananda.mp3", "Journey In Satchidananda", "100 Greatest", "Various Artists", "Alice Coltrane Pharoah Sanders"),
			entry:     PlaylistEntry{TrackName: "Journey In Satchidananda", Album: "Journey in Satchidananda", Artists: []string{"Alice Coltrane", "Pharoah Sanders"}},
			wantMatch: true,
		},
		{
			// Negative regression: even with fuzzy matching enabled,
			// "Mort Garson" must NOT funnel into the Dana and Alden
			// "Dragonfly" file, because the artists share no
			// word-aligned substring at all.
			name:      "Mort Garson entry still does NOT match Dana and Alden under fuzzy gate",
			track:     makeLocalTrack("Dana and Alden/Quiet Music/Dana and Alden - Dragonfly.mp3", "Dragonfly", "Quiet Music for Young People", "Dana and Alden"),
			entry:     PlaylistEntry{TrackName: "Dragonfly", Album: "Dragonfly", Artists: []string{"Mort Garson"}},
			wantMatch: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			idx := makeIndex([]*localTrack{tc.track})
			candidate, reason := idx.findBestMatch(tc.entry)
			if tc.wantMatch && candidate == nil {
				t.Fatalf("expected match, got nil (reason=%q)", reason)
			}
			if !tc.wantMatch && candidate != nil {
				t.Fatalf("expected miss, got %q (reason=%q)", candidate.Path, reason)
			}
		})
	}
}

// TestArtistOnlyFiresWithTokenOverlap confirms the artist-only fallback
// still catches the legitimate case it's meant for: same artist, slightly
// different title where at least one title token is shared but neither
// word-aligned containment nor a high-jaccard token-similarity fires.
func TestArtistOnlyFiresWithTokenOverlap(t *testing.T) {
	track := makeLocalTrack("Artist/Album/Cosmic Drift.mp3", "Cosmic Drift", "Album", "Artist")
	idx := makeIndex([]*localTrack{track})

	// Different surrounding words, but the shared token "cosmic" is enough
	// for the artist-only path to fire.
	entry := PlaylistEntry{TrackName: "Cosmic Wanderer Returns Home", Artists: []string{"Artist"}}
	candidate, reason := idx.findBestMatch(entry)
	if candidate == nil {
		t.Fatalf("expected match via artist-only with token overlap, got nil (reason=%q)", reason)
	}
	if candidate.Path != track.Path {
		t.Fatalf("matched %q, want %q (reason=%q)", candidate.Path, track.Path, reason)
	}
	if !strings.Contains(reason, "artist") {
		t.Fatalf("reason = %q, want to mention artist-only path", reason)
	}
}
