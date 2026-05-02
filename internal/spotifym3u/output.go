package spotifym3u

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func WritePlaylist(outputPath, targetPrefix string, matches []MatchResult) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := fmt.Fprintln(f, "#EXTM3U"); err != nil {
		return err
	}

	cleanPrefix := strings.TrimRight(targetPrefix, "/")
	for _, match := range matches {
		artist := match.Candidate.Artist
		title := match.Candidate.Title
		if title == "" {
			title = match.Request.TrackName
		}
		entryPath := match.Candidate.Path
		if cleanPrefix != "" {
			entryPath = cleanPrefix + "/" + entryPath
		}
		entryPath = filepath.ToSlash(entryPath)
		if _, err := fmt.Fprintf(f, "#EXTINF:%d,%s - %s\n%s\n", match.Request.Duration, artist, title, entryPath); err != nil {
			return err
		}
	}

	return nil
}

func PrintSummary(outputPath, targetPrefix string, summary MatchSummary, dryRun, debug bool) {
	fmt.Printf("Processed %d playlist entries\n", summary.Total)
	fmt.Printf("Matched %d entries\n", summary.Matched)
	fmt.Printf("Missing %d entries\n", summary.Missing)
	if dryRun {
		fmt.Printf("Dry run: playlist file was not written\n")
	} else {
		fmt.Printf("Playlist written to %s\n", outputPath)
		if targetPrefix != "" {
			fmt.Printf("Target prefix: %s\n", targetPrefix)
		}
	}
	if debug && summary.Missing > 0 {
		fmt.Println("\nUnmatched tracks:")
		for _, entry := range summary.Unmatched {
			fmt.Printf("- %s | %s | %s\n", entry.TrackName, entry.Artist, entry.Album)
		}
	}
}
