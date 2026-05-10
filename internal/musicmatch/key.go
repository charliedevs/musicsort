package musicmatch

import "strings"

// BuildKey joins parts with a unit-separator (0x1f) into a single string
// that's safe to use as a map key. The 0x1f delimiter is chosen because it
// can't appear in normalized titles/artists/albums (NormalizeForMatch
// strips control characters along with the rest of the punctuation), so
// adjacent fields can never collide across rows.
func BuildKey(parts ...string) string {
	return strings.Join(parts, "\x1f")
}
