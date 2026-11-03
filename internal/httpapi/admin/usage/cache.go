package usage

import "strings"

func usageETag(revision string) string {
	return `W/"usage-` + revision + `"`
}

// Conditional GET uses weak comparison, including a list of validators.
func usageETagMatches(header, tag string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || strings.TrimPrefix(candidate, "W/") == strings.TrimPrefix(tag, "W/") {
			return true
		}
	}
	return false
}
