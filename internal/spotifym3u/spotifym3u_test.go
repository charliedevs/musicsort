package spotifym3u

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeForMatch(t *testing.T) {
	input := "  Beyoncé - Halo (Live)!!! "
	want := "beyoncé halo live"
	got := NormalizeForMatch(input)
	if got != want {
		t.Fatalf("NormalizeForMatch(%q) = %q, want %q", input, got, want)
	}
}

func TestBuildIndexAndMatch(t *testing.T) {
	index := &TrackIndex{
		exactMatch:  make(map[string][]*localTrack),
		titleArtist: make(map[string][]*localTrack),
		titleOnly:   make(map[string][]*localTrack),
	}

	track := &localTrack{
		Path:         "artist/album/song.mp3",
		Title:        "Song",
		Artist:       "Some Artist",
		Album:        "Album Name",
		NormTitle:    NormalizeForMatch("Song"),
		NormArtist:   NormalizeForMatch("Some Artist"),
		NormAlbum:    NormalizeForMatch("Album Name"),
		NormFilename: NormalizeForMatch("song.mp3"),
	}
	index.allTracks = append(index.allTracks, track)
	index.exactMatch[buildKey(track.NormTitle, track.NormArtist, track.NormAlbum)] = []*localTrack{track}
	index.titleArtist[buildKey(track.NormTitle, track.NormArtist, "")] = []*localTrack{track}
	index.titleOnly[track.NormTitle] = []*localTrack{track}

	entries := []PlaylistEntry{{TrackName: "Song", Artist: "Some Artist", Album: "Album Name", Duration: 213}}
	result := index.MatchPlaylist(entries)
	if result.Matched != 1 {
		t.Fatalf("expected 1 match, got %d", result.Matched)
	}
	if result.Matches[0].Candidate.Path != track.Path {
		t.Fatalf("matched wrong track, got %q", result.Matches[0].Candidate.Path)
	}
}

func TestParseSpotifyCSV(t *testing.T) {
	const csvContent = `URI,Track Name,Album,Artist,Release,Duration (ms)
spotify:track:1,Test Song,Test Album,Test Artist,2024,180000
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
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].TrackName != "Test Song" || entries[0].Artist != "Test Artist" {
		t.Fatalf("unexpected entry: %+v", entries[0])
	}
}
