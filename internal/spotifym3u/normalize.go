package spotifym3u

import (
	"regexp"
	"strings"
)

var normalizePattern = regexp.MustCompile(`[^
\p{L}\p{N}\s]+`)

func NormalizeForMatch(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = normalizePattern.ReplaceAllString(value, " ")
	return strings.Join(strings.Fields(value), " ")
}

func buildKey(parts ...string) string {
	return strings.Join(parts, "\x1f")
}
