package spotifym3u

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"musicsort/internal/clioutput"
	"musicsort/internal/musicmatch"
)

// WritePlaylist writes an extended M3U file at outputPath containing one
// #EXTINF/#path entry per match, in input order. targetPrefix (when set) is
// prepended to each track's relative path so playlists generated for a
// portable device land on the right mount point.
//
// On any write failure the partially-written file is removed so callers
// don't end up with a corrupt playlist on disk.
func WritePlaylist(outputPath, targetPrefix string, matches []MatchResult) (err error) {
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer func() {
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(outputPath)
		}
	}()

	if _, err = fmt.Fprintln(f, "#EXTM3U"); err != nil {
		return err
	}

	cleanPrefix := strings.TrimRight(targetPrefix, "/")
	for _, match := range matches {
		artist := "Unknown Artist"
		if len(match.Candidate.Artists) > 0 {
			artist = match.Candidate.Artists[0]
		}
		title := match.Candidate.Title
		if title == "" {
			title = match.Entry.TrackName
		}
		entryPath := match.Candidate.Path
		if cleanPrefix != "" {
			entryPath = cleanPrefix + "/" + entryPath
		}
		entryPath = filepath.ToSlash(entryPath)
		if _, err = fmt.Fprintf(f, "#EXTINF:%d,%s - %s\n%s\n",
			match.Entry.Duration, artist, title, entryPath); err != nil {
			return err
		}
	}
	return nil
}

// PrintSummary writes the post-run summary block to stdout, followed by the
// debug breakdown of unmatched entries when debug is true.
func PrintSummary(outputPath, targetPrefix string, summary MatchSummary, dryRun, debug bool) {
	matchedLabel := "Matched:"
	missingLabel := "Missing:"
	resultText := "completed successfully"
	if dryRun {
		matchedLabel = "Would match:"
		missingLabel = "Would be missing:"
		resultText = "dry run (playlist file was not written)"
	}

	clioutput.SummaryHeader("Summary")
	clioutput.SummaryItem("Processed:", summary.Total)
	clioutput.SummaryStatus(matchedLabel, summary.Matched, clioutput.Green)
	clioutput.SummaryStatus(missingLabel, summary.Missing, clioutput.Red)
	clioutput.SummaryItem("Result:", resultText)
	clioutput.SummaryItem("Playlist:", outputPath)
	if targetPrefix != "" {
		clioutput.SummaryItem("Target prefix:", targetPrefix)
	}

	if debug && summary.Missing > 0 {
		clioutput.Newline()
		printUnmatched(summary)
	}
}

// printUnmatched prints each unmatched entry along with its normalized
// lookup keys and the closest near-miss candidates from the source index.
// This is the "why didn't this match?" view that --debug produces.
func printUnmatched(summary MatchSummary) {
	clioutput.InfoLine("Unmatched tracks:")
	diagByIndex := make(map[int]MissDiagnostic, len(summary.Diagnostics))
	for _, d := range summary.Diagnostics {
		diagByIndex[d.Entry.Index] = d
	}
	for _, entry := range summary.Unmatched {
		artist := "Unknown"
		if len(entry.Artists) > 0 {
			artist = entry.Artists[0]
		}
		clioutput.InfoLine("- %s | %s | %s", entry.TrackName, artist, entry.Album)
		normArtist := ""
		if len(entry.Artists) > 0 {
			normArtist = musicmatch.NormalizeForMatch(entry.Artists[0])
		}
		clioutput.InfoLine("    looked up: title=%q artist=%q album=%q",
			musicmatch.NormalizeForMatch(entry.TrackName), normArtist, musicmatch.NormalizeForMatch(entry.Album))

		diag, ok := diagByIndex[entry.Index]
		if !ok || len(diag.Closest) == 0 {
			clioutput.InfoLine("    closest: (no near-miss candidates in source)")
			continue
		}
		clioutput.InfoLine("    closest:")
		for _, c := range diag.Closest {
			clioutput.InfoLine("      [%.2f] %s | title=%q artist=%q album=%q",
				c.Overlap, c.Track.Path, c.Track.NormTitle,
				strings.Join(c.Track.NormArtists, ", "), c.Track.NormAlbum)
		}
	}
}
