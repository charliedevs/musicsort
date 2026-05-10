package spotifym3u

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"musicsort/internal/musicmatch"
)

// PlaylistEntry is a single row from the input CSV after column aliasing
// and per-field cleanup. Index is the 1-based row number among accepted
// rows (i.e. rows that actually contributed an entry); it is used to key
// MissDiagnostic entries back to the original row.
type PlaylistEntry struct {
	Index     int
	URI       string
	TrackName string
	Album     string
	Artists   []string
	Release   string
	Duration  int
}

// ParseSpotifyCSV reads an Exportify or hand-rolled Spotify CSV file at path
// and returns one PlaylistEntry per non-empty data row. The header row is
// matched case-insensitively, and a few common column-name variants are
// accepted (e.g. "Track Name" / "Track" / "Title", "Artist Name(s)" /
// "Artists" / "Artist"). Rows whose track name resolves to the empty string
// after trimming are silently skipped.
func ParseSpotifyCSV(path string) ([]PlaylistEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.TrimLeadingSpace = true
	r.FieldsPerRecord = -1

	head, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read csv header: %w", err)
	}

	headers := make(map[string]int, len(head))
	for i, name := range head {
		headers[strings.ToLower(strings.TrimSpace(name))] = i
	}

	getIndex := func(keys ...string) int {
		for _, key := range keys {
			if idx, ok := headers[strings.ToLower(key)]; ok {
				return idx
			}
		}
		return -1
	}

	trackNameIndex := getIndex("track name", "track", "title")
	if trackNameIndex == -1 {
		return nil, fmt.Errorf("csv header missing track name column")
	}

	// Exportify uses "Album Name" / "Artist Name(s)" / "Release Date" /
	// "Track URI"; older or hand-rolled CSVs may use the shorter forms. Try
	// the long names first so an Exportify export resolves on the first
	// lookup.
	albumIndex := getIndex("album name", "album")
	artistIndex := getIndex("artist name(s)", "artist names", "artists", "artist name", "artist")
	releaseIndex := getIndex("release date", "release")
	durationIndex := getIndex("duration (ms)", "duration_ms", "duration")
	uriIndex := getIndex("track uri", "spotify uri", "uri", "url")

	var entries []PlaylistEntry
	// row tracks the source line number for error messages; the first data
	// row sits on line 2 because line 1 is the header.
	for row := 2; ; row++ {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read csv row %d: %w", row, err)
		}
		if len(record) == 0 {
			continue
		}

		trackName := getField(record, trackNameIndex)
		if strings.TrimSpace(trackName) == "" {
			continue
		}

		duration := 0
		if durationIndex >= 0 {
			duration = parseDurationSeconds(record[durationIndex])
		}

		entries = append(entries, PlaylistEntry{
			Index:     len(entries) + 1,
			URI:       getField(record, uriIndex),
			TrackName: trackName,
			Album:     getField(record, albumIndex),
			Artists:   musicmatch.SplitArtists(getField(record, artistIndex)),
			Release:   getField(record, releaseIndex),
			Duration:  duration,
		})
	}

	return entries, nil
}

// getField returns the trimmed value at idx, or "" if idx is out of range.
// idx == -1 (column not present in header) is the common case for optional
// fields.
func getField(record []string, idx int) string {
	if idx < 0 || idx >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[idx])
}

// parseDurationSeconds parses a millisecond duration string and returns it
// as an integer number of seconds. Empty or unparseable values yield 0
// rather than an error; M3U's #EXTINF directive treats 0 as "unknown",
// which is appropriate for missing data.
func parseDurationSeconds(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if ms, err := strconv.Atoi(value); err == nil {
		return ms / 1000
	}
	return 0
}
