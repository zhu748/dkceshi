package tooldiagnostics

import (
	"fmt"
	"strings"

	"ds2api/internal/tooldiagnostics"
)

func diagnosticETag(summary tooldiagnostics.Summary) string {
	return fmt.Sprintf(`"%s-%d-%s"`, summary.ID, summary.Revision, summary.DetailVersion)
}

func diagnosticETagMatches(header, tag string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || strings.TrimPrefix(candidate, "W/") == tag {
			return true
		}
	}
	return false
}
