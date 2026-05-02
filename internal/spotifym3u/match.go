package spotifym3u

import "strings"

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

func (index *TrackIndex) MatchPlaylist(entries []PlaylistEntry) MatchSummary {
	var summary MatchSummary
	summary.Total = len(entries)

	for _, entry := range entries {
		match, reason := index.findBestMatch(entry)
		if match != nil {
			summary.Matched++
			summary.Matches = append(summary.Matches, MatchResult{Request: entry, Candidate: match, Reason: reason})
			continue
		}

		summary.Missing++
		summary.Unmatched = append(summary.Unmatched, entry)
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
