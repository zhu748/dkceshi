package responsehistory

import (
	"net/http"
	"time"

	"ds2api/internal/chathistory"
)

type progressSnapshot struct {
	thinking, content string
	valid             bool
}

// Status events and upstream keepalives may repeat the same output. They must
// not rewrite a large history file just to advance its elapsed time. Failed
// writes retain the previous snapshot and retry after the normal interval.
func (s *Session) Progress(thinking, content string) {
	if s == nil || s.store == nil || s.disabled {
		return
	}
	previous := s.lastProgress
	if previous.valid && previous.thinking == thinking && previous.content == content {
		return
	}
	now := time.Now()
	if !progressPersistDue(now, s.lastPersist) {
		return
	}
	s.lastPersist = now
	if s.persistUpdate(chathistory.UpdateParams{
		Status: "streaming", ReasoningContent: thinking, Content: content,
		StatusCode: http.StatusOK, ElapsedMs: time.Since(s.startedAt).Milliseconds(),
	}) {
		s.lastProgress = progressSnapshot{thinking: thinking, content: content, valid: true}
	}
}
