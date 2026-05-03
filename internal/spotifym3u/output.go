package spotifym3u

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"musicsort/internal/clioutput"
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
	matchedLabel := "Matched:"
	missingLabel := "Missing:"
	if dryRun {
		matchedLabel = "Would match:"
		missingLabel = "Would be missing:"
	}

	clioutput.SummaryHeader("Summary")
	clioutput.SummaryItem("Processed:", summary.Total)
	clioutput.SummaryStatus(matchedLabel, summary.Matched, clioutput.Green)
	clioutput.SummaryStatus(missingLabel, summary.Missing, clioutput.Red)
	if dryRun {
		clioutput.SummaryItem("Result:", "dry run (playlist file was not written)")
		clioutput.SummaryItem("Playlist:", outputPath)
	} else {
		clioutput.SummaryItem("Result:", "completed successfully")
		clioutput.SummaryItem("Playlist:", outputPath)
		if targetPrefix != "" {
			clioutput.SummaryItem("Target prefix:", targetPrefix)
		}
	}
	if debug && summary.Missing > 0 {
		clioutput.Newline()
		clioutput.InfoLine("Unmatched tracks:")
		for _, entry := range summary.Unmatched {
			clioutput.InfoLine("- %s | %s | %s", entry.TrackName, entry.Artist, entry.Album)
		}
	}
}
