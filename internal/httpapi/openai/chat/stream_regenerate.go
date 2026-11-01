package chat

import "ds2api/internal/toolstream"

// Only called after the runtime deferred an error with no delivered answer.
// Preserve reasoning already streamed, but discard malformed tool fragments,
// old message IDs and error state before parsing a replacement generation.
func (s *chatStreamRuntime) resetForRegeneration() {
	s.toolCallsPending = false
	s.accumulator.RawText.Reset()
	s.accumulator.Text.Reset()
	s.accumulator.RawThinking.Reset()
	s.accumulator.ToolDetectionThinking.Reset()
	s.accumulator.ResetThinkingGuard()
	s.accumulator.ResetContentPolicy()
	s.deferredText.Reset()
	s.toolSieve = toolstream.State{DeferMalformed: true, Marker: s.toolSieve.Marker}
	s.responseMessageID = 0
	s.upstreamErr = ""
	s.finalErrorStatus, s.finalErrorMessage, s.finalErrorCode = 0, "", ""
}
