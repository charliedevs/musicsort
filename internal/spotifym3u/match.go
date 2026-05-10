package spotifym3u

import (
	"fmt"
	"sort"

	"musicsort/internal/clioutput"
	"musicsort/internal/musicmatch"
)

// MatchResult pairs a successfully-matched playlist entry with the local
// track it resolved to and a short human-readable reason for the match.
type MatchResult struct {
	Entry     PlaylistEntry
	Candidate *localTrack
	Reason    string
}

// MissDiagnostic captures the closest local tracks for an entry that failed
// to match, used by --debug output to make root-cause analysis easy.
type MissDiagnostic struct {
	Entry   PlaylistEntry
	Closest []DiagnosticCandidate
}

// DiagnosticCandidate is a single near-miss, scored by Jaccard token overlap
// against the entry's title.
type DiagnosticCandidate struct {
	Track   *localTrack
	Overlap float64
}

// MatchSummary is the aggregate result of matching every CSV entry against
// the local track index.
type MatchSummary struct {
	Total       int
	Matched     int
	Missing     int
	Matches     []MatchResult
	Unmatched   []PlaylistEntry
	Diagnostics []MissDiagnostic
}

// Scoring constants. Larger == better; ties are broken alphabetically by
// path inside pickBest. The gaps between tiers are intentional: a stronger
// match (e.g. title+artist+album) must always beat any weaker one even if
// several weaker paths fire for the same candidate.
const (
	scoreExact     = 100
	scoreTitleArt  = 80
	scoreTitleAlb  = 70
	scoreTitleHash = 40
	scoreTitleSub  = 30
	scoreTokenSim  = 25
	scoreArtist    = 20
	scoreFilename  = 10
)

// tokenOverlapThreshold is the minimum Jaccard similarity required for the
// token-similarity fallback to accept a candidate.
const tokenOverlapThreshold = 0.6

// MatchPlaylist resolves every entry to a local track when possible, also
// recording per-miss diagnostics for unmatched entries.
func (index *TrackIndex) MatchPlaylist(entries []PlaylistEntry, verbose bool) MatchSummary {
	summary := MatchSummary{Total: len(entries)}

	for i, entry := range entries {
		match, reason := index.findBestMatch(entry)
		if match != nil {
			summary.Matched++
			summary.Matches = append(summary.Matches, MatchResult{Entry: entry, Candidate: match, Reason: reason})
			if verbose {
				clioutput.InfoLine("%s %d/%d %s -> %s (%s)",
					clioutput.Label("MATCH", clioutput.Green), i+1, summary.Total,
					entry.TrackName, match.Path, reason)
			}
		} else {
			summary.Missing++
			summary.Unmatched = append(summary.Unmatched, entry)
			summary.Diagnostics = append(summary.Diagnostics, MissDiagnostic{
				Entry:   entry,
				Closest: index.closestCandidates(entry, 3),
			})
			if verbose {
				clioutput.InfoLine("%s %d/%d %s (%s)",
					clioutput.Label("MISS", clioutput.Red), i+1, summary.Total,
					entry.TrackName, reason)
			}
		}

		if !verbose {
			clioutput.ProgressLine("Matching %d/%d entries...", i+1, summary.Total)
		}
	}
	if !verbose && clioutput.ProgressEnabled() {
		clioutput.Newline()
	}

	return summary
}

// candidateInfo is the per-track scoring record we accumulate while running
// findBestMatch. Each track ends up with the *best* (reason, score) pair it
// earned across all stages.
type candidateInfo struct {
	track  *localTrack
	reason string
	score  int
}

// entryContext bundles the per-entry derived fields we'd otherwise recompute
// across multiple matching stages.
type entryContext struct {
	normTitle   string
	normAlbum   string
	titleTokens map[string]struct{}
	// artistNorms is the deduped, normalized artist list. firstArtist is the
	// first non-empty entry of artistNorms (or "" if there are none).
	artistNorms []string
	firstArtist string
	hasArtist   bool
}

// buildEntryContext computes everything findBestMatch needs about an entry
// exactly once, so individual matching stages can read it cheaply.
func buildEntryContext(entry PlaylistEntry) entryContext {
	ctx := entryContext{
		normTitle: musicmatch.NormalizeForMatch(entry.TrackName),
		normAlbum: musicmatch.NormalizeForMatch(entry.Album),
	}
	ctx.titleTokens = musicmatch.TokenSet(ctx.normTitle)
	ctx.artistNorms = musicmatch.NormalizeEntryArtists(entry.Artists)
	ctx.hasArtist = len(ctx.artistNorms) > 0
	if ctx.hasArtist {
		ctx.firstArtist = ctx.artistNorms[0]
	}
	return ctx
}

// findBestMatch runs the entry through every scoring stage, accumulates
// candidate hits, and returns the highest-scoring track. Returns (nil,
// reason) if no stage produced a candidate.
func (index *TrackIndex) findBestMatch(entry PlaylistEntry) (*localTrack, string) {
	ctx := buildEntryContext(entry)
	if ctx.normTitle == "" {
		return nil, "empty track name"
	}

	candidates := make(map[string]candidateInfo)
	upsert := func(track *localTrack, reason string, score int) {
		if existing, exists := candidates[track.Path]; !exists || existing.score < score {
			candidates[track.Path] = candidateInfo{track, reason, score}
		}
	}

	index.collectArtistKeyedHits(ctx, entry.Artists, upsert)
	index.collectTitleAlbumHits(ctx, upsert)
	index.collectTitleOnlyHits(ctx, entry.Artists, upsert)
	index.collectTitleSubstringHits(ctx, entry.Artists, upsert)
	if bestScore(candidates) < scoreTitleSub {
		index.collectTokenSimilarityHits(ctx, entry.Artists, upsert)
	}
	index.collectArtistOnlyHits(ctx, entry.Artists, upsert)
	index.collectFilenameHits(ctx, entry.Artists, upsert)

	if len(candidates) == 0 {
		return nil, "no match"
	}
	return pickBest(candidates)
}

// collectArtistKeyedHits handles the strongest stages: full triple-key match
// (title+artist+album, score 100) and the title+artist hash (score 80).
func (index *TrackIndex) collectArtistKeyedHits(ctx entryContext, rawArtists []string, upsert func(*localTrack, string, int)) {
	for i, artist := range rawArtists {
		na := musicmatch.NormalizeForMatch(artist)
		if na == "" {
			continue
		}
		for _, track := range index.exactMatch[musicmatch.BuildKey(ctx.normTitle, na, ctx.normAlbum)] {
			upsert(track, fmt.Sprintf("title+artist+album[%d]", i), scoreExact)
		}
		for _, track := range index.titleArtist[musicmatch.BuildKey(ctx.normTitle, na, "")] {
			upsert(track, fmt.Sprintf("title+artist[%d]", i), scoreTitleArt)
		}
	}
}

// collectTitleAlbumHits emits title+album hash hits (score 70). Skipped when
// the entry has no album info, since the corresponding hash key would be
// degenerate (title+empty-album).
func (index *TrackIndex) collectTitleAlbumHits(ctx entryContext, upsert func(*localTrack, string, int)) {
	if ctx.normAlbum == "" {
		return
	}
	for _, track := range index.titleAlbum[musicmatch.BuildKey(ctx.normTitle, "", ctx.normAlbum)] {
		upsert(track, "title+album", scoreTitleAlb)
	}
}

// collectTitleOnlyHits emits title-only hash hits (score 40). Common titles
// like "Time", "Blue", "Intro", "Dragonfly", "Forever" exist on tracks by
// many different artists, so when the CSV row carries artist info we only
// emit candidates that *also* share an artist with the entry. Without this
// gate, those titles fan out across whatever single same-titled file the
// user happens to have.
func (index *TrackIndex) collectTitleOnlyHits(ctx entryContext, rawArtists []string, upsert func(*localTrack, string, int)) {
	for _, track := range index.titleOnly[ctx.normTitle] {
		if ctx.hasArtist && !musicmatch.SharesArtist(track.NormArtists, rawArtists) {
			continue
		}
		upsert(track, "title", scoreTitleHash)
	}
}

// collectTitleSubstringHits emits hits where one normalized title is a
// word-aligned substring of the other (score 30). It's the most error-prone
// fallback because short local titles like "As" or "Blue" character-substring
// match into long unrelated CSV titles ("Ask Me No Questions", "Asshtonpark").
// Gated on:
//
//   - word-aligned containment with a minimum inner length, and
//   - when the CSV row has artist info, on a shared artist.
func (index *TrackIndex) collectTitleSubstringHits(ctx entryContext, rawArtists []string, upsert func(*localTrack, string, int)) {
	for _, track := range index.allTracks {
		if track.NormTitle == "" {
			continue
		}
		if !musicmatch.WordAlignedSubstring(track.NormTitle, ctx.normTitle) {
			continue
		}
		if ctx.hasArtist && !musicmatch.SharesArtist(track.NormArtists, rawArtists) {
			continue
		}
		upsert(track, "title-substring", scoreTitleSub)
	}
}

// collectTokenSimilarityHits emits Jaccard-token-overlap hits (score 25).
// Only invoked when the stronger paths haven't produced anything at or
// above scoreTitleSub, to avoid spending O(N) work on entries we already
// have a strong match for.
func (index *TrackIndex) collectTokenSimilarityHits(ctx entryContext, rawArtists []string, upsert func(*localTrack, string, int)) {
	for _, track := range index.allTracks {
		overlap := musicmatch.Jaccard(ctx.titleTokens, track.TitleTokens)
		if overlap < tokenOverlapThreshold {
			continue
		}
		// Only accept token-similarity matches when the entry has no
		// artist info or the candidate shares at least one normalized
		// artist with the entry.
		if ctx.firstArtist != "" && !musicmatch.SharesArtist(track.NormArtists, rawArtists) {
			continue
		}
		upsert(track, fmt.Sprintf("token-similarity(%.2f)", overlap), scoreTokenSim)
	}
}

// collectArtistOnlyHits emits artist-keyed hits (score 20), which is the
// weakest hash-based path. Without a gate it pulls in every track by an
// artist whose specific song is missing, so we require at least one shared
// title token.
func (index *TrackIndex) collectArtistOnlyHits(ctx entryContext, rawArtists []string, upsert func(*localTrack, string, int)) {
	for i, artist := range rawArtists {
		na := musicmatch.NormalizeForMatch(artist)
		if na == "" {
			continue
		}
		for _, track := range index.artistOnly[na] {
			if !musicmatch.ShareTitleToken(ctx.titleTokens, track.TitleTokens) {
				continue
			}
			upsert(track, fmt.Sprintf("artist[%d]", i), scoreArtist)
		}
	}
}

// collectFilenameHits emits filename-based hits (score 10). Uses the same
// word-aligned containment and shared-artist gating as the title-substring
// fallback so an entry like "Witch" by Alex G doesn't match
// `Nardo Wick - Wicked Witch.mp3` purely on filename text overlap.
func (index *TrackIndex) collectFilenameHits(ctx entryContext, rawArtists []string, upsert func(*localTrack, string, int)) {
	for _, track := range index.allTracks {
		if track.NormFilename == "" {
			continue
		}
		if !musicmatch.WordAlignedSubstring(track.NormFilename, ctx.normTitle) {
			continue
		}
		if ctx.hasArtist && !musicmatch.SharesArtist(track.NormArtists, rawArtists) {
			continue
		}
		upsert(track, "filename", scoreFilename)
	}
}

// pickBest returns the highest-scoring candidate, breaking ties by Path so
// output is deterministic across runs.
func pickBest(candidates map[string]candidateInfo) (*localTrack, string) {
	infos := make([]candidateInfo, 0, len(candidates))
	for _, info := range candidates {
		infos = append(infos, info)
	}
	sort.Slice(infos, func(i, j int) bool {
		if infos[i].score != infos[j].score {
			return infos[i].score > infos[j].score
		}
		return infos[i].track.Path < infos[j].track.Path
	})
	return infos[0].track, infos[0].reason
}

// bestScore returns the highest score currently in the candidate map, or 0
// if the map is empty. Used to short-circuit the token-similarity stage.
func bestScore(candidates map[string]candidateInfo) int {
	best := 0
	for _, info := range candidates {
		if info.score > best {
			best = info.score
		}
	}
	return best
}

// closestCandidates returns up to n local tracks with the highest title-token
// overlap with the entry, used by --debug output to suggest near-misses.
func (index *TrackIndex) closestCandidates(entry PlaylistEntry, n int) []DiagnosticCandidate {
	if len(index.allTracks) == 0 || n <= 0 {
		return nil
	}
	entryTokens := musicmatch.TokenSet(musicmatch.NormalizeForMatch(entry.TrackName))
	if len(entryTokens) == 0 {
		return nil
	}
	scored := make([]DiagnosticCandidate, 0, len(index.allTracks))
	for _, t := range index.allTracks {
		overlap := musicmatch.Jaccard(entryTokens, t.TitleTokens)
		if overlap == 0 {
			continue
		}
		scored = append(scored, DiagnosticCandidate{Track: t, Overlap: overlap})
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].Overlap != scored[j].Overlap {
			return scored[i].Overlap > scored[j].Overlap
		}
		return scored[i].Track.Path < scored[j].Track.Path
	})
	if len(scored) > n {
		scored = scored[:n]
	}
	return scored
}
