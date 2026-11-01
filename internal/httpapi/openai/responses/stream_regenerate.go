package responses

import "ds2api/internal/toolstream"

func (s *responsesStreamRuntime) resetForRegeneration() {
	s.toolCallsPending = false
	s.accumulator.RawText.Reset()
	s.accumulator.Text.Reset()
	s.accumulator.RawThinking.Reset()
	s.accumulator.ToolDetectionThinking.Reset()
	s.accumulator.ResetThinkingGuard()
	s.accumulator.ResetContentPolicy()
	s.deferredText.Reset()
	s.sieve = toolstream.State{CleanVisible: true, DeferMalformed: true, Marker: s.sieve.Marker}
	s.responseMessageID = 0
	s.upstreamErr = ""
	s.finalErrorStatus, s.finalErrorMessage, s.finalErrorCode = 0, "", ""
}
