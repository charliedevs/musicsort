package spotifym3u

import (
	"fmt"
	"strings"

	"musicsort/internal/clioutput"
)

type MatchResult struct {
	Request   PlaylistEntry
	Candidate *localTrack
	Reason    string
}

type MatchSummary struct {
	Total     int
	Matched   int
	Missing   int
	Matches   []MatchResult
	Unmatched []PlaylistEntry
}

func (index *TrackIndex) MatchPlaylist(entries []PlaylistEntry, verbose bool) MatchSummary {
	var summary MatchSummary
	summary.Total = len(entries)

	for i, entry := range entries {
		match, reason := index.findBestMatch(entry)
		if match != nil {
			summary.Matched++
			summary.Matches = append(summary.Matches, MatchResult{Request: entry, Candidate: match, Reason: reason})
			if verbose {
				clioutput.InfoLine("%s %d/%d %s -> %s (%s)", clioutput.Label("MATCH", clioutput.Green), i+1, summary.Total, entry.TrackName, match.Path, reason)
			} else {
				clioutput.ProgressLine("Matching %d/%d entries...", i+1, summary.Total)
			}
			continue
		}

		summary.Missing++
		summary.Unmatched = append(summary.Unmatched, entry)
		if verbose {
			clioutput.InfoLine("%s %d/%d %s (%s)", clioutput.Label("MISS", clioutput.Red), i+1, summary.Total, entry.TrackName, reason)
		} else {
			clioutput.ProgressLine("Matching %d/%d entries...", i+1, summary.Total)
		}
	}
	if !verbose {
		fmt.Println()
	}

	return summary
}

func (index *TrackIndex) findBestMatch(entry PlaylistEntry) (*localTrack, string) {
	normTitle := NormalizeForMatch(entry.TrackName)
	if normTitle == "" {
		return nil, "empty track name"
	}
	artistKey := NormalizeForMatch(entry.Artist)
	albumKey := NormalizeForMatch(entry.Album)

	if candidate := first(index.exactMatch[buildKey(normTitle, artistKey, albumKey)]); candidate != nil {
		return candidate, "title+artist+album"
	}

	if artistKey != "" {
		if candidate := first(index.titleArtist[buildKey(normTitle, artistKey, "")]); candidate != nil {
			return candidate, "title+artist"
		}
	}

	if candidate := first(index.titleOnly[normTitle]); candidate != nil {
		return candidate, "title"
	}

	for _, candidate := range index.allTracks {
		if strings.Contains(candidate.NormFilename, normTitle) || strings.Contains(candidate.NormTitle, normTitle) {
			return candidate, "filename"
		}
	}

	return nil, "no match"
}

func first(list []*localTrack) *localTrack {
	if len(list) == 0 {
		return nil
	}
	return list[0]
}
