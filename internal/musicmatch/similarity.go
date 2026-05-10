package musicmatch

import (
	"strings"
	"unicode/utf8"
)

// MinSubstringRunes is the minimum length (in runes) of the inner string
// required for the title-substring / filename fallbacks to fire. Anything
// shorter has too high a collision rate against unrelated CSV titles
// (the local title "As" character-substring-matches into "Ask Me No
// Questions", "Asshtonpark", etc.). 4 is the smallest value that still
// catches legitimate four-letter song titles like "Halo".
const MinSubstringRunes = 4

// MinArtistSubRunes is the minimum length (in runes) of the shorter
// artist string for the fuzzy artist substring path to fire.
const MinArtistSubRunes = 5

// TokenSet builds a set of whitespace-separated tokens from a normalized
// string, used by the Jaccard fallback in findBestMatch and by the
// closest-candidate diagnostic.
func TokenSet(normalized string) map[string]struct{} {
	if normalized == "" {
		return nil
	}
	fields := strings.Fields(normalized)
	if len(fields) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(fields))
	for _, t := range fields {
		set[t] = struct{}{}
	}
	return set
}

// WordAlignedSubstring reports whether either of the two strings is a
// word-aligned substring of the other, with the inner string at least
// MinSubstringRunes long. Either string may be the haystack; whichever
// is shorter is treated as the needle.
//
// "Word-aligned" means the needle is bordered on both sides by either
// the start/end of the haystack or an ASCII space (the only word
// separator NormalizeForMatch emits). This rules out the dangerous
// character-level containment that lets "as" match "ask".
func WordAlignedSubstring(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return utf8.RuneCountInString(a) >= MinSubstringRunes
	}
	needle, haystack := a, b
	if utf8.RuneCountInString(b) < utf8.RuneCountInString(a) {
		needle, haystack = b, a
	}
	if utf8.RuneCountInString(needle) < MinSubstringRunes {
		return false
	}
	return containsAtWordBoundary(haystack, needle)
}

// containsAtWordBoundary returns true if needle appears in haystack with
// a space (or the start/end of the string) on both sides.
func containsAtWordBoundary(haystack, needle string) bool {
	if len(haystack) < len(needle) {
		return false
	}
	for offset := 0; ; {
		i := strings.Index(haystack[offset:], needle)
		if i < 0 {
			return false
		}
		abs := offset + i
		end := abs + len(needle)
		leftOK := abs == 0 || haystack[abs-1] == ' '
		rightOK := end == len(haystack) || haystack[end] == ' '
		if leftOK && rightOK {
			return true
		}
		offset = abs + 1
		if offset > len(haystack)-len(needle) {
			return false
		}
	}
}

// ShareTitleToken reports whether the entry and candidate share at least
// one normalized title token. Used to gate the artist-only fallback so
// that distinct CSV rows with no token overlap (e.g. "Born Under a Bad
// Sign", "Voodoo Woman", "Back To The River") don't all collapse onto
// the same alphabetically-first track for an artist.
func ShareTitleToken(entry, track map[string]struct{}) bool {
	if len(entry) == 0 || len(track) == 0 {
		return false
	}
	for tok := range entry {
		if _, ok := track[tok]; ok {
			return true
		}
	}
	return false
}

// Jaccard returns |A ∩ B| / |A ∪ B|. Empty sets return 0.
func Jaccard(a, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter := 0
	for k := range a {
		if _, ok := b[k]; ok {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// SharesArtist reports whether any of the local track's normalized artists
// matches any of the entry's normalized artists, either strictly or after a
// word-aligned substring check.
//
// The strict path (exact equality on a normalized artist) is what catches
// the common case where a CSV row's artist matches a file's tag verbatim.
// The fuzzy path (word-aligned containment, minimum MinArtistSubRunes
// runes on the shorter side) catches three real-world tagging
// inconsistencies that show up across the test corpora:
//
//   - "Wings" (CSV) vs. "Paul McCartney And Wings" (file album-artist),
//     because SplitArtists deliberately does *not* split on " and "/" & "
//     to preserve names like "King Gizzard & The Lizard Wizard";
//   - "Emancipator" + "Cloudchord" (Spotify ;-list) vs. one combined file
//     artist "Emancipator & Cloudchord" (single tag value, & stripped);
//   - "Alice Coltrane" (CSV) vs. a comp file tagged
//     "Various Artists, Alice Coltrane Pharoah Sanders".
//
// MinArtistSubRunes is intentionally a touch higher (5) than the title
// substring threshold (4) so common 3-4 letter artist words ("the",
// "and", "king", "love") don't fuzzy-match every artist that contains
// them.
//
// entryArtists is accepted as raw (unnormalized) strings for caller
// convenience; it is normalized here.
func SharesArtist(localNorms []string, entryArtists []string) bool {
	if len(localNorms) == 0 || len(entryArtists) == 0 {
		return false
	}
	entryNorms := NormalizeEntryArtists(entryArtists)
	if len(entryNorms) == 0 {
		return false
	}
	for _, la := range localNorms {
		if la == "" {
			continue
		}
		for _, ea := range entryNorms {
			if la == ea {
				return true
			}
			if artistFuzzyContains(la, ea) || artistFuzzyContains(ea, la) {
				return true
			}
		}
	}
	return false
}

// NormalizeEntryArtists folds raw entry artists to their normalized form,
// dropping empties.
func NormalizeEntryArtists(entryArtists []string) []string {
	out := make([]string, 0, len(entryArtists))
	for _, a := range entryArtists {
		if na := NormalizeForMatch(a); na != "" {
			out = append(out, na)
		}
	}
	return out
}

// artistFuzzyContains reports whether short is a word-aligned substring of
// long, with short being at least MinArtistSubRunes runes long. This
// catches "wings" inside "paul mccartney and wings" and
// "emancipator" inside "emancipator cloudchord", while declining to fire
// for shorter common words like "the", "and", "king".
func artistFuzzyContains(long, short string) bool {
	if utf8.RuneCountInString(short) < MinArtistSubRunes {
		return false
	}
	if utf8.RuneCountInString(long) <= utf8.RuneCountInString(short) {
		return false
	}
	return containsAtWordBoundary(long, short)
}
