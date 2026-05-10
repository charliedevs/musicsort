// Package musicmatch contains pure normalization and similarity helpers
// shared by the spotifym3u and musicsort tools. It has no I/O dependencies
// so it can be safely linked into both CLIs without dragging in CLI-output
// or filesystem code.
package musicmatch

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// punctuationPattern collapses every run of non-letter / non-digit / non-whitespace
// characters down to a single space.
var punctuationPattern = regexp.MustCompile(`[^\p{L}\p{N}\s]+`)

// dashSuffixKeywords are markers that indicate a version annotation when they
// appear after " - " in a title. We deliberately keep this list conservative:
// bare common nouns like "single", "original", "bonus", "deluxe" are excluded
// because they collide with real song titles ("Beyonce - Single Ladies").
// Multi-word forms ("single version", "original mix", "bonus track") are
// listed before any prefix they share so the regex prefers the longer match.
var dashSuffixKeywords = []string{
	`radio edit`,
	`radio mix`,
	`extended mix`,
	`extended`,
	`single version`,
	`single edit`,
	`original mix`,
	`bonus track`,
	`deluxe edition`,
	`anniversary edition`,
	`expanded edition`,
	`remastered`,
	`remaster`,
	`remix`,
	`acoustic`,
	`live`,
	`instrumental`,
	`demo`,
	`version`,
	`edit`,
	`mono`,
	`stereo`,
	`feat\..*`,
	`ft\..*`,
	`featuring.*`,
}

// parenSuffixKeywords additionally accept bare single-word descriptors that
// are unambiguous when they appear inside parentheses/brackets, even if they
// would be too common to strip after a bare " - ".
var parenSuffixKeywords = append(append([]string{}, dashSuffixKeywords...),
	`single`,
	`original`,
	`bonus`,
	`deluxe`,
	`clean`,
	`explicit`,
	`reissue`,
	`with .*`,
)

// parenSuffix strips trailing parenthesised/bracketed annotations whose interior
// matches one of the version keywords, e.g. "Halo (Live)", "Song (Original Mix)".
var parenSuffix = regexp.MustCompile(`(?i)\s*[\(\[][^)\]]*?(?:` + strings.Join(parenSuffixKeywords, `|`) + `)[^)\]]*[\)\]]\s*`)

// dashSuffix strips " - <keyword>..." trailing a title, e.g. "Halo - Live",
// "Song - 2010 Remaster". Anchored to end-of-string so we don't accidentally
// remove " - " in the middle of a title.
var dashSuffix = regexp.MustCompile(`(?i)\s+-\s+[^-]*?(?:` + strings.Join(dashSuffixKeywords, `|`) + `)[^-]*$`)

// trailingFeat strips a free-floating "feat./ft./featuring/with X ..." trail
// that is not wrapped in parens/brackets.
var trailingFeat = regexp.MustCompile(`(?i)\s+(?:feat\.?|ft\.?|featuring|with)\s+.*$`)

// artistSplit captures the separators we accept between collaborating artists.
// We deliberately keep this conservative: only `;` (the Spotify/Exportify
// convention for joining multiple artists) and whitespace-bounded word
// separators (feat./ft./featuring/with/vs). Splitting on `&`, `,`, `/`, `+`,
// or bare `x` would shred legitimate single-artist names like
// "King Gizzard & The Lizard Wizard", "Crosby, Stills, Nash & Young",
// "Hall & Oates", "30/70", or "Masashi Kitamura + Phonogenix" that show up
// frequently in real Exportify exports.
var artistSplit = regexp.MustCompile(`(?i)\s*;\s*|\s+(?:feat\.?|ft\.?|featuring|with|vs\.?)\s+`)

// folderArtistSplit is a more aggressive split used only when picking the
// primary artist for a folder name. In that context a literal `/` in the
// artist tag (e.g. "$uicideboy$/GERM") would otherwise survive into the
// filesystem as a path separator (which Sanitize then crushes into `_`,
// producing eyesores like "$uicideboy$_GERM"). It's intentionally NOT used
// for matching, because real artist names sometimes contain `/` ("30/70")
// and we don't want to fragment those during indexing.
var folderArtistSplit = regexp.MustCompile(`(?i)\s*[;/]\s*|\s+(?:feat\.?|ft\.?|featuring|with|vs\.?)\s+`)

// AlbumYearSuffix matches a trailing "(YYYY)" year suffix on an album folder
// or album name, e.g. "Faces (2014)". It is exported so callers (notably
// musicsort's TargetIndex) can peel a year off an existing folder name
// before normalizing it for dedup.
var AlbumYearSuffix = regexp.MustCompile(`\s*\(\d{4}\)\s*$`)

// newDiacriticFolder returns a Transformer that strips Unicode combining
// marks (Mn) so "Beyoncé" folds to "Beyonce". Callers must NOT share a
// single Chain across goroutines: golang.org/x/text/transform buffers
// internally and concurrent transform.String on one Chain corrupts state
// (slice bounds panics). musicsort runs metadata folding from many worker
// goroutines, so each foldDiacritics invocation builds its own Chain.
func newDiacriticFolder() transform.Transformer {
	return transform.Chain(
		norm.NFD,
		runes.Remove(runes.In(unicode.Mn)),
		norm.NFC,
	)
}

// NormalizeForMatch produces a canonical lower-cased, diacritic-folded,
// punctuation-stripped form used as a hash key for matching.
func NormalizeForMatch(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = foldDiacritics(value)

	for {
		next := parenSuffix.ReplaceAllString(value, " ")
		next = dashSuffix.ReplaceAllString(next, "")
		next = trailingFeat.ReplaceAllString(next, "")
		if next == value {
			break
		}
		value = next
	}

	value = punctuationPattern.ReplaceAllString(value, " ")
	return strings.Join(strings.Fields(value), " ")
}

// foldDiacritics decomposes accented characters into base+combining form
// (NFD) and drops the combining marks, so "Beyoncé" -> "beyonce".
func foldDiacritics(s string) string {
	out, _, err := transform.String(newDiacriticFolder(), s)
	if err != nil {
		return s
	}
	return out
}

// SplitArtists breaks an artist string from a tag or CSV cell into individual
// artist names, splitting on common collaboration separators and trimming
// duplicate/empty entries.
func SplitArtists(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := artistSplit.Split(value, -1)
	return dedupeNonEmpty(parts)
}

// PrimaryArtistForFolder returns the leading artist from a tag value, suitable
// for use as a top-level folder name. Unlike SplitArtists, this also splits
// on a literal `/` so an artist tag like "$uicideboy$/GERM" yields the
// primary "$uicideboy$" rather than letting Sanitize crush the slash into
// "_". Callers that need the matching-grade split (which preserves names
// containing `/`) should keep using SplitArtists.
func PrimaryArtistForFolder(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	parts := folderArtistSplit.Split(value, -1)
	cleaned := dedupeNonEmpty(parts)
	if len(cleaned) == 0 {
		return ""
	}
	return cleaned[0]
}

// dedupeNonEmpty trims, drops empties, and removes diacritic-folded
// case-insensitive duplicates while preserving original ordering. Returns a
// nil slice (rather than a zero-length one) when nothing usable remains, so
// callers can keep using `len(out) == 0` and direct nil comparisons.
func dedupeNonEmpty(parts []string) []string {
	seen := make(map[string]struct{}, len(parts))
	var out []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(foldDiacritics(trimmed))
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}
