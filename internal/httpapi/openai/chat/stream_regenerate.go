package chat

import (
	"ds2api/internal/assistantturn"
	"ds2api/internal/toolstream"
)

// Called after a deferred failure; delivered tool preambles remain visible.
// Preserve reasoning already streamed, but discard malformed tool fragments,
// old message IDs and error state before parsing a replacement generation.
func (s *chatStreamRuntime) resetForRegeneration() {
	s.toolRecovery.Start(s.finalErrorCode, s.visibleTextEmitted)
	s.diagnosticProgress = assistantturn.DiagnosticProgress{}
	s.toolCallsPending = false
	s.accumulator.RawText.Reset()
	s.accumulator.Text.Reset()
	s.accumulator.RawThinking.Reset()
	s.accumulator.ToolDetectionThinking.Reset()
	s.accumulator.ResetThinkingGuard()
	s.accumulator.ResetContentPolicy()
	s.deferredText.Reset()
	s.leadingText.Reset()
	s.toolSieve = toolstream.State{CleanVisible: true, DeferMalformed: true, StripReferenceMarkers: s.stripReferenceMarkers, Marker: s.toolSieve.Marker}
	s.responseMessageID = 0
	s.upstreamErr = ""
	s.finalErrorStatus, s.finalErrorMessage, s.finalErrorCode = 0, "", ""
}
