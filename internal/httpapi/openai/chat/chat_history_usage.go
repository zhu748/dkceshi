package chat

import (
	"net/http"
	"time"

	"ds2api/internal/chathistory"
	"ds2api/internal/promptcompat"
	"ds2api/internal/usagestats"
)

func (s *chatHistorySession) stopped(thinking, content, finishReason string, current ...promptcompat.StandardRequest) {
	if s == nil {
		return
	}
	request := s.usageRequest
	if request.PromptTokenText == "" && request.FinalPrompt == "" {
		request.FinalPrompt = s.finalPrompt
	}
	if request.ResolvedModel == "" {
		request.ResolvedModel = s.recordModel
	}
	usage := usagestats.PartialUsage(request, thinking, content, current...)
	s.stoppedWithUsage(thinking, content, finishReason, usage)
}

func (s *chatHistorySession) stoppedWithUsage(thinking, content, finishReason string, usage map[string]any) {
	if s == nil {
		return
	}
	if usage == nil {
		s.stopped(thinking, content, finishReason)
		return
	}
	s.recordUsage(usage)
	if s.store == nil || s.disabled {
		return
	}
	s.persistUpdate(chathistory.UpdateParams{
		Status:           "stopped",
		ReasoningContent: thinking,
		Content:          content,
		StatusCode:       http.StatusOK,
		ElapsedMs:        time.Since(s.startedAt).Milliseconds(),
		FinishReason:     finishReason,
		Usage:            usage,
		Completed:        true,
	})
}
