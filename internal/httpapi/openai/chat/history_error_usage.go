package chat

import (
	"ds2api/internal/chathistory"
	"ds2api/internal/promptcompat"
	"ds2api/internal/tokenusage"
	"ds2api/internal/usagestats"
	"time"
)

func (s *chatHistorySession) errorWithUpstream(status int, message, reason, thinking, content string, observed tokenusage.Snapshot, current ...promptcompat.StandardRequest) {
	if s == nil {
		return
	}
	var usage map[string]any
	if thinking != "" || content != "" || len(observed.Messages) > 0 {
		usage = usagestats.PartialUsageWithUpstream(s.usageRequest, thinking, content, observed, current...)
	}
	s.error(status, message, reason, thinking, content, usage)
}

func (s *chatHistorySession) error(statusCode int, message, finishReason, thinking, content string, usage ...map[string]any) {
	if s == nil || s.store == nil || s.disabled {
		return
	}
	var reported map[string]any
	if len(usage) > 0 {
		reported = usage[0]
	}
	s.persistUpdate(chathistory.UpdateParams{
		Status:           "error",
		Usage:            reported,
		ReasoningContent: thinking,
		Content:          content,
		Error:            message,
		StatusCode:       statusCode,
		ElapsedMs:        time.Since(s.startedAt).Milliseconds(),
		FinishReason:     finishReason,
		Completed:        true,
	})
}
