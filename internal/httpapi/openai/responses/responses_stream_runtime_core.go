package responses

import (
	"context"
	"net/http"
	"strings"

	"ds2api/internal/assistantturn"
	"ds2api/internal/config"
	openaifmt "ds2api/internal/format/openai"
	"ds2api/internal/promptcompat"
	"ds2api/internal/responsehistory"
	"ds2api/internal/sse"
	streamengine "ds2api/internal/stream"
	"ds2api/internal/toolcall"
	"ds2api/internal/toolstream"
)

type responsesStreamRuntime struct {
	w        http.ResponseWriter
	rc       *http.ResponseController
	canFlush bool

	responseID    string
	model         string
	finalPrompt   string
	refFileTokens int
	toolNames     []string
	toolsRaw      any
	traceID       string
	toolChoice    promptcompat.ToolChoicePolicy

	thinkingEnabled       bool
	searchEnabled         bool
	stripReferenceMarkers bool

	bufferToolContent    bool
	toolCallsEmitted     bool
	toolCallsPending     bool
	toolCallsDoneEmitted bool

	deferredText         strings.Builder
	sieve                toolstream.State
	accumulator          assistantturn.StreamAccumulator
	visibleText          strings.Builder
	visibleReasoning     strings.Builder
	responseMessageID    int
	upstreamErr          string
	streamToolCallIDs    map[int]string
	functionItemIDs      map[int]string
	functionOutputIDs    map[int]int
	functionArgs         map[int]string
	functionDone         map[int]bool
	functionAdded        map[int]bool
	functionNames        map[int]string
	messageItemID        string
	messageOutputID      int
	nextOutputID         int
	messageAdded         bool
	messagePartAdded     bool
	reasoningItemID      string
	reasoningOutputIndex int
	reasoningItemAdded   bool
	reasoningPartAdded   bool
	reasoningClosed      bool
	sequence             int
	failed               bool
	finalErrorStatus     int
	finalErrorMessage    string
	finalErrorCode       string

	persistResponse func(obj map[string]any)
	history         *responsehistory.Session

	// toolCallRepair / toolCallRepairCtx wire the phase-3 finalize-only LLM
	// tool-call repair into the streaming finalize path (M4). Nil unless the
	// handler enabled repair; the per-chunk sieve never receives them.
	toolCallRepair    toolcall.ToolCallRepairInvoker
	toolCallRepairCtx context.Context
}

func newResponsesStreamRuntime(
	w http.ResponseWriter,
	rc *http.ResponseController,
	canFlush bool,
	responseID string,
	model string,
	finalPrompt string,
	thinkingEnabled bool,
	searchEnabled bool,
	stripReferenceMarkers bool,
	toolNames []string,
	toolsRaw any,
	bufferToolContent bool,
	_ bool, // retained for callers; tool batches are validated before delivery
	toolChoice promptcompat.ToolChoicePolicy,
	traceID string,
	persistResponse func(obj map[string]any),
	history *responsehistory.Session,
) *responsesStreamRuntime {
	return &responsesStreamRuntime{
		w:                     w,
		rc:                    rc,
		canFlush:              canFlush,
		responseID:            responseID,
		model:                 model,
		finalPrompt:           finalPrompt,
		thinkingEnabled:       thinkingEnabled,
		searchEnabled:         searchEnabled,
		stripReferenceMarkers: stripReferenceMarkers,
		toolNames:             toolNames,
		toolsRaw:              toolsRaw,
		bufferToolContent:     bufferToolContent,
		sieve:                 toolstream.State{DeferMalformed: true},
		streamToolCallIDs:     map[int]string{},
		functionItemIDs:       map[int]string{},
		functionOutputIDs:     map[int]int{},
		functionArgs:          map[int]string{},
		functionDone:          map[int]bool{},
		functionAdded:         map[int]bool{},
		functionNames:         map[int]string{},
		messageOutputID:       -1,
		reasoningOutputIndex:  -1,
		toolChoice:            toolChoice,
		traceID:               traceID,
		persistResponse:       persistResponse,
		history:               history,
		visibleReasoning:      strings.Builder{},
		accumulator: assistantturn.StreamAccumulator{
			Prompt:                finalPrompt,
			ThinkingEnabled:       thinkingEnabled,
			SearchEnabled:         searchEnabled,
			StripReferenceMarkers: stripReferenceMarkers,
		},
	}
}

func (s *responsesStreamRuntime) failResponse(status int, message, code string) {
	s.failed = true
	s.finalErrorStatus = status
	s.finalErrorMessage = message
	s.finalErrorCode = code
	failedResp := map[string]any{
		"id":          s.responseID,
		"type":        "response",
		"object":      "response",
		"model":       s.model,
		"status":      "failed",
		"status_code": status,
		"output":      []any{},
		"output_text": "",
		"error": map[string]any{
			"message": message,
			"type":    openAIErrorType(status),
			"code":    code,
			"param":   nil,
		},
	}
	if s.persistResponse != nil {
		s.persistResponse(failedResp)
	}
	if s.history != nil {
		s.history.Error(status, message, code, responsehistory.ThinkingForArchive(s.accumulator.RawThinking.String(), s.accumulator.ToolDetectionThinking.String(), s.accumulator.Thinking.String()), responsehistory.TextForArchive(s.accumulator.RawText.String(), s.accumulator.Text.String()))
	}
	s.sendEvent("response.failed", openaifmt.BuildResponsesFailedPayload(s.responseID, s.model, status, message, code))
	s.sendDone()
}

func (s *responsesStreamRuntime) markContextCancelled() {
	s.failed = true
	s.finalErrorStatus = 499
	s.finalErrorMessage = "request context cancelled"
	s.finalErrorCode = string(streamengine.StopReasonContextCancelled)
}

func (s *responsesStreamRuntime) logToolPolicyRejections(textParsed toolcall.ToolCallParseResult) {
	logRejected := func(parsed toolcall.ToolCallParseResult, channel string) {
		rejected := filteredRejectedToolNamesForLog(parsed.RejectedToolNames)
		if !parsed.RejectedByPolicy || len(rejected) == 0 {
			return
		}
		config.Logger.Warn(
			"[responses] rejected tool calls by policy",
			"trace_id", strings.TrimSpace(s.traceID),
			"channel", channel,
			"tool_choice_mode", s.toolChoice.Mode,
			"rejected_tool_names", strings.Join(rejected, ","),
		)
	}
	logRejected(textParsed, "text")
}

func (s *responsesStreamRuntime) onParsed(parsed sse.LineResult) streamengine.ParsedDecision {
	if !parsed.Parsed {
		return streamengine.ParsedDecision{}
	}
	if parsed.ResponseMessageID > 0 {
		s.responseMessageID = parsed.ResponseMessageID
	}
	if parsed.ErrorMessage != "" {
		s.upstreamErr = parsed.ErrorMessage
		return streamengine.ParsedDecision{Stop: true, StopReason: streamengine.StopReason("upstream_error")}
	}
	if parsed.ContentFilter {
		return streamengine.ParsedDecision{Stop: true, StopReason: streamengine.StopReason("content_filter")}
	}
	if parsed.Stop {
		return streamengine.ParsedDecision{Stop: true}
	}

	batch := responsesDeltaBatch{runtime: s}
	s.accumulator.ToolMarker = s.sieve.Marker
	accumulated := s.accumulator.Apply(parsed)
	for _, p := range accumulated.Parts {
		if p.Type == "thinking" {
			batch.append("reasoning", p.VisibleText)
			continue
		}
		if p.RawText == "" {
			continue
		}
		if p.CitationOnly {
			continue
		}
		if !s.bufferToolContent {
			batch.append("text", p.VisibleText)
			continue
		}
		batch.flush()
		s.processToolStreamEvents(toolstream.ProcessChunk(&s.sieve, p.RawText, s.toolNames), true, true)
	}

	batch.flush()
	if s.history != nil {
		s.history.Progress(
			responsehistory.ThinkingForArchive(s.accumulator.RawThinking.String(), s.accumulator.ToolDetectionThinking.String(), s.accumulator.Thinking.String()),
			responsehistory.TextForArchive(s.accumulator.RawText.String(), s.accumulator.Text.String()),
		)
	}
	// 上游内容规则（复读死循环 / 编造的 `[Tool]:` 角色标记）在此统一收口：
	// 结束读取，保留已生成内容走正常收尾。
	return streamengine.StopForCut(accumulated.Cut, "responses", "responses", accumulated.ContentSeen)
}
