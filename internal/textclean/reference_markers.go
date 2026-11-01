package textclean

import (
	"regexp"
	"strings"
)

var citationReferenceMarkerPattern = regexp.MustCompile(`(?i)\[(citation|reference):\s*\d+\]`)

func StripReferenceMarkers(text string) string {
	if text == "" {
		return text
	}
	return citationReferenceMarkerPattern.ReplaceAllString(text, "")
}

// StripReferenceMarkersEnabled returns the default for streaming surfaces,
// where partial citation/reference markers are hidden before the final
// link metadata is available.
func StripReferenceMarkersEnabled() bool {
	return true
}

func referenceDisplayUnit(raw string, final bool) (end int, wait bool) {
	if raw[0] != '[' {
		return 0, false
	}
	if loc := citationReferenceMarkerPattern.FindStringIndex(raw); loc != nil && loc[0] == 0 {
		return loc[1], false
	}
	if final || len(raw) > 64 {
		return 0, false
	}
	lower := strings.ToLower(raw)
	for _, prefix := range []string{"[citation:", "[reference:"} {
		if strings.HasPrefix(prefix, lower) {
			return 0, true
		}
		if strings.HasPrefix(lower, prefix) {
			tail := strings.TrimLeft(lower[len(prefix):], " \t\r\n")
			if strings.Trim(tail, "0123456789") == "" {
				return 0, true
			}
		}
	}
	return 0, false
}
