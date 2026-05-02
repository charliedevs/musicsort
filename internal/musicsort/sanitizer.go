package musicsort

import "regexp"

var illegalChars = regexp.MustCompile(`[/\\?%*:|"<>]+`)

// Sanitize removes or replaces illegal characters from a string for use in filenames.
func Sanitize(s string) string {
	return illegalChars.ReplaceAllString(s, "_")
}
