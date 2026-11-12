package responses

import (
	"ds2api/internal/assistantturn"
	"ds2api/internal/toolstream"
)

func (s *responsesStreamRuntime) resetForRegeneration() {
	s.toolRecovery.Start(s.finalErrorCode, s.visibleText.Len() > 0)
	s.diagnosticProgress = assistantturn.DiagnosticProgress{}
	s.toolCallsPending = false
	s.accumulator.UpstreamUsage.Request = nil
	s.accumulator.UpstreamUsage.Messages = nil
	s.accumulator.UpstreamUsage.Truncated = false
	s.accumulator.RawText.Reset()
	s.accumulator.Text.Reset()
	s.accumulator.RawThinking.Reset()
	s.accumulator.ToolDetectionThinking.Reset()
	s.accumulator.ResetThinkingGuard()
	s.accumulator.ResetContentPolicy()
	s.deferredText.Reset()
	s.leadingText.Reset()
	s.sieve = toolstream.State{CleanVisible: true, DeferMalformed: true, StripReferenceMarkers: s.stripReferenceMarkers, Marker: s.sieve.Marker}
	s.responseMessageID = 0
	s.upstreamErr = ""
	s.finalErrorStatus, s.finalErrorMessage, s.finalErrorCode = 0, "", ""
}

func (s *responsesStreamRuntime) prepareAnswerText(content string) string {
	return s.leadingText.Write(content)
}
