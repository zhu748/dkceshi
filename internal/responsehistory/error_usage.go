package responsehistory

import (
	"ds2api/internal/promptcompat"
	"ds2api/internal/tokenusage"
	"ds2api/internal/usagestats"
)

// Failed output retains observed usage for inspection without billing it as a
// successful consumption entry. Errors before generation keep usage unknown.
func (s *Session) ErrorWithUpstream(status int, message, reason, thinking, content string, observed tokenusage.Snapshot, current ...promptcompat.StandardRequest) {
	if s == nil {
		return
	}
	var usage map[string]any
	if thinking != "" || content != "" || len(observed.Messages) > 0 {
		usage = usagestats.PartialUsageWithUpstream(s.usageRequest, thinking, content, observed, current...)
	}
	s.Error(status, message, reason, thinking, content, usage)
}
