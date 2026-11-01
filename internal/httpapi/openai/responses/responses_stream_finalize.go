package responses

import (
	"ds2api/internal/assistantturn"
	"ds2api/internal/completionruntime"
	openaifmt "ds2api/internal/format/openai"
	"ds2api/internal/responsehistory"
	"ds2api/internal/sse"
	"ds2api/internal/toolstream"
	"net/http"
	"strings"
)

func (s *responsesStreamRuntime) finalize(finishReason string, deferEmptyOutput bool) bool {
	if s.accumulator.Cut() == sse.CutNone {
		s.onParsed(sse.LineResult{Parsed: true, Flush: true})
	}
	s.failed = false
	s.finalErrorStatus = 0
	s.finalErrorMessage = ""
	s.finalErrorCode = ""
	if s.bufferToolContent {
		s.processToolStreamEvents(toolstream.Flush(&s.sieve, s.toolNames), true, true)
	}

	finalThinking := s.accumulator.Thinking.String()
	finalToolDetectionThinking := s.accumulator.ToolDetectionThinking.String()
	finalText := s.accumulator.Text.String()
	turn := assistantturn.BuildTurnFromStreamSnapshot(assistantturn.StreamSnapshot{
		ContentCut:            s.accumulator.Cut(),
		RawText:               s.accumulator.RawText.String(),
		VisibleText:           finalText,
		RawThinking:           s.accumulator.RawThinking.String(),
		VisibleThinking:       finalThinking,
		DetectionThinking:     finalToolDetectionThinking,
		ContentFilter:         finishReason == "content_filter",
		UpstreamError:         s.upstreamErr,
		ResponseMessageID:     s.responseMessageID,
		AlreadyEmittedCalls:   s.toolCallsEmitted,
		AlreadyEmittedToolRaw: s.toolCallsDoneEmitted,
	}, assistantturn.BuildOptions{
		Model:                 s.model,
		Prompt:                s.finalPrompt,
		RefFileTokens:         s.refFileTokens,
		SearchEnabled:         s.searchEnabled,
		StripReferenceMarkers: s.stripReferenceMarkers,
		ToolNames:             s.toolNames,
		ToolsRaw:              s.toolsRaw,
		ToolChoice:            s.toolChoice,
		ToolCallRepair:        s.toolCallRepair,
		ToolCallRepairCtx:     s.toolCallRepairCtx,
		ToolMarker:            s.sieve.Marker,
	})
	textParsed := turn.ParsedToolCalls
	detected := turn.ToolCalls
	s.logToolPolicyRejections(textParsed)
	if turn.Error != nil {
		if completionruntime.CanDeferOutputError(turn.Error, deferEmptyOutput, strings.TrimSpace(s.visibleText.String()) != "" || s.toolCallsEmitted || s.toolCallsDoneEmitted) {
			s.finalErrorStatus, s.finalErrorMessage, s.finalErrorCode = turn.Error.Status, turn.Error.Message, turn.Error.Code
			return false
		}
		s.closeReasoningItem()
		s.closeMessageItem()
		s.failResponse(turn.Error.Status, turn.Error.Message, turn.Error.Code)
		return true
	}

	if len(detected) > 0 {
		s.toolCallsEmitted = true
		if !s.toolCallsDoneEmitted {
			s.emitFunctionCallDoneEvents(detected)
		}
	}

	if len(detected) == 0 && s.deferredText.Len() > 0 {
		s.emitTextDelta(cleanVisibleOutput(toolstream.FilterVisible(s.deferredText.String(), ""), s.stripReferenceMarkers))
		s.deferredText.Reset()
	}
	s.closeReasoningItem()
	s.closeMessageItem()

	outcome := assistantturn.FinalizeTurn(turn, assistantturn.FinalizeOptions{
		AlreadyEmittedToolCalls: s.toolCallsEmitted || s.toolCallsDoneEmitted,
	})
	if outcome.ShouldFail {
		status, message, code := outcome.Error.Status, outcome.Error.Message, outcome.Error.Code
		if completionruntime.CanDeferOutputError(outcome.Error, deferEmptyOutput, strings.TrimSpace(s.visibleText.String()) != "" || s.toolCallsEmitted || s.toolCallsDoneEmitted) {
			s.finalErrorStatus = status
			s.finalErrorMessage = message
			s.finalErrorCode = code
			return false
		}
		s.failResponse(status, message, code)
		return true
	}
	s.closeIncompleteFunctionItems()

	obj := s.buildCompletedResponseObject(turn.Thinking, turn.Text, detected)
	if s.persistResponse != nil {
		s.persistResponse(obj)
	}
	if s.history != nil {
		s.history.Success(
			http.StatusOK,
			responsehistory.ThinkingForArchive(turn.RawThinking, turn.DetectionThinking, turn.Thinking),
			responsehistory.TextForArchive(turn.RawText, turn.Text),
			outcome.FinishReason,
			assistantturn.OpenAIResponsesUsage(turn),
		)
	}
	s.sendEvent("response.completed", openaifmt.BuildResponsesCompletedPayload(obj))
	s.sendDone()
	return true
}
