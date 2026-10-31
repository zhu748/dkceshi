package claude

import (
	"ds2api/internal/responsehistory"
	"ds2api/internal/toolstream"
)

func (s *claudeStreamRuntime) resetForRegeneration() {
	s.toolCallsPending = false
	s.rawText.Reset()
	s.text.Reset()
	s.rawThinking.Reset()
	s.toolDetectionThinking.Reset()
	s.deferredText.Reset()
	s.resetContentPolicy()
	s.sieve = toolstream.State{DeferMalformed: true, Marker: s.sieve.Marker}
	s.responseMessageID = 0
	s.upstreamErr = ""
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
