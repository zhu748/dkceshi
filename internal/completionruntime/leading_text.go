package completionruntime

import "strings"

// LeadingText delays whitespace until an actual answer commits the stream.
// Regeneration can then discard an empty attempt without leaving stale text
// or closing a reasoning item. Whitespace after commitment streams normally.
type LeadingText struct {
	pending   strings.Builder
	committed bool
}

func (s *LeadingText) Write(text string) string {
	if s.committed {
		return text
	}
	s.pending.WriteString(text)
	if strings.TrimSpace(text) == "" {
		return ""
	}
	s.committed = true
	text = s.pending.String()
	s.pending.Reset()
	return text
}

func (s *LeadingText) Reset() {
	s.pending.Reset()
	s.committed = false
}
