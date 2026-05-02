package spotifym3u

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

type PlaylistEntry struct {
	Index     int
	URI       string
	TrackName string
	Album     string
	Artist    string
	Release   string
	Duration  int
}

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

	headers := make(map[string]int)
	for i, name := range head {
		header := strings.ToLower(strings.TrimSpace(name))
		headers[header] = i
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

	albumIndex := getIndex("album")
	artistIndex := getIndex("artist")
	releaseIndex := getIndex("release")
	durationIndex := getIndex("duration (ms)", "duration_ms", "duration")
	uriIndex := getIndex("uri", "url")

	var entries []PlaylistEntry
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
			duration = parseDuration(record[durationIndex])
		}

		entries = append(entries, PlaylistEntry{
			Index:     len(entries) + 1,
			URI:       getField(record, uriIndex),
			TrackName: trackName,
			Album:     getField(record, albumIndex),
			Artist:    getField(record, artistIndex),
			Release:   getField(record, releaseIndex),
			Duration:  duration,
		})
	}

	return entries, nil
}

func getField(record []string, idx int) string {
	if idx < 0 || idx >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[idx])
}

func parseDuration(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if ms, err := strconv.Atoi(value); err == nil {
		return ms / 1000
	}
	return 0
}
