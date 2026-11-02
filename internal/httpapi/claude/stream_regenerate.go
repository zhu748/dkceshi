package claude

import (
	"ds2api/internal/completionruntime"
	"ds2api/internal/responsehistory"
	"ds2api/internal/sse"
	"ds2api/internal/toolstream"
)

func (s *claudeStreamRuntime) rejectedOutput() string {
	return completionruntime.ToolRetrySource(s.rawText.String(), s.toolDetectionThinking.String())
}

func (s *claudeStreamRuntime) resetForRegeneration() {
	s.toolCallsPending = false
	s.rawText.Reset()
	s.text.Reset()
	s.rawThinking.Reset()
	s.thinkingGuard = toolstream.VisibleGuard{Marker: s.sieve.Marker}
	s.toolDetectionThinking.Reset()
	s.deferredText.Reset()
	s.resetContentPolicy()
	s.sieve = toolstream.State{CleanVisible: true, DeferMalformed: true, StripReferenceMarkers: s.stripReferenceMarkers, Marker: s.sieve.Marker}
	s.responseMessageID = 0
	s.upstreamErr = ""
	s.contentFilter = false
	s.finalErrorStatus, s.finalErrorMessage, s.finalErrorCode = 0, "", ""
}

func (s *claudeStreamRuntime) failRetry(status int, message, code string) {
	s.ended = true
	s.finalErrorStatus, s.finalErrorMessage, s.finalErrorCode = status, message, code
	s.closeThinkingBlock()
	s.closeTextBlock()
	if s.history != nil {
		s.history.Error(status, message, code, responsehistory.ThinkingForArchive(s.rawThinking.String(), s.toolDetectionThinking.String(), s.thinking.String()), responsehistory.TextForArchive(s.rawText.String(), s.text.String()))
	}
	s.sendErrorWithCode(status, message, code)
}

// resetContentPolicy starts a replacement generation; continuations retain it.
func (s *claudeStreamRuntime) resetContentPolicy() {
	if s == nil {
		return
	}
	s.policy = sse.ContentPolicy{Prompt: s.promptTokenText}
}

func (s *claudeStreamRuntime) allowOutput(n int) bool {
	return s.policy.AllowOutputSize(s.rawText.Len() + s.rawThinking.Len() + s.toolDetectionThinking.Len() + n)
}
