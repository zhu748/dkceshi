package gemini

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"ds2api/internal/assistantturn"
	"ds2api/internal/auth"
	"ds2api/internal/completionruntime"
	"ds2api/internal/config"
	dsprotocol "ds2api/internal/deepseek/protocol"
	"ds2api/internal/outputformat"
	"ds2api/internal/promptcompat"
	"ds2api/internal/responsehistory"
	"ds2api/internal/sse"
	streamengine "ds2api/internal/stream"
	"ds2api/internal/streamwriter"
	"ds2api/internal/toolcall"
	"ds2api/internal/toolstream"
	"ds2api/internal/util"
)

//nolint:unused // retained for native Gemini stream handling path.
func (h *Handler) handleStreamGenerateContent(w http.ResponseWriter, r *http.Request, resp *http.Response, model, finalPrompt string, thinkingEnabled, searchEnabled bool, toolNames []string, toolsRaw any, historySessions ...*responsehistory.Session) {
	var historySession *responsehistory.Session
	if len(historySessions) > 0 {
		historySession = historySessions[0]
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if detail := completionruntime.TryDetectCaptchaFromBody(body); detail != "" {
			config.Logger.Warn("[gemini_stream] captcha challenge detected on initial response", "detail", detail)
		}
		if historySession != nil {
			historySession.Error(resp.StatusCode, strings.TrimSpace(string(body)), "error", "", "")
		}
		writeGeminiError(w, resp.StatusCode, strings.TrimSpace(string(body)))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	rc := http.NewResponseController(w)
	_, canFlush := w.(http.Flusher)
	// Without a normalized request, stream prose through tool interception.
	runtime := newGeminiStreamRuntime(w, rc, canFlush, model, finalPrompt, thinkingEnabled, searchEnabled, stripReferenceMarkersEnabled(), toolNames, toolsRaw, false, historySession)

	initialType := "text"
	if thinkingEnabled {
		initialType = "thinking"
	}
	streamengine.ConsumeSSE(streamengine.ConsumeConfig{
		Context:             r.Context(),
		Body:                resp.Body,
		ThinkingEnabled:     thinkingEnabled,
		InitialType:         initialType,
		KeepAliveInterval:   time.Duration(dsprotocol.KeepAliveTimeout) * time.Second,
		IdleTimeout:         time.Duration(dsprotocol.StreamIdleTimeout) * time.Second,
		MaxKeepAliveNoInput: dsprotocol.MaxKeepaliveCount,
	}, streamengine.ConsumeHooks{
		OutputError: runtime.output.Err,
		OnParsed:    runtime.onParsed,
		OnFinalize: func(_ streamengine.StopReason, _ error) {
			runtime.finalize(false)
		},
	})
}

//nolint:unused // retained for native Gemini stream handling path.
type geminiStreamRuntime struct {
	toolRecovery       completionruntime.ToolRecovery
	diagnosticProgress assistantturn.DiagnosticProgress
	output             streamwriter.Writer
	leadingText        completionruntime.LeadingText
	w                  http.ResponseWriter
	rc                 *http.ResponseController
	canFlush           bool

	model       string
	finalPrompt string
	// inputTokenCounts memoizes the prompt-side BPE count per prompt text so
	// finalize (and each retry attempt's finalize) does not re-encode the same
	// hundreds-of-KB prompt.
	inputTokenCounts map[string]int
	refFileTokens    int

	thinkingEnabled bool
	searchEnabled   bool
	// Prose streams through the sieve; required/forced policies alone hold it.
	// Every tool batch waits for shared final validation.
	toolSieve             toolstream.State
	deferredText          strings.Builder
	toolCallsPending      bool
	answerDelivered       bool
	toolsDelivered        bool
	bufferContent         bool
	stripReferenceMarkers bool
	toolNames             []string
	toolsRaw              any
	accumulator           *assistantturn.Accumulator
	contentFilter         bool
	upstreamErr           string
	responseMessageID     int
	finalErrorStatus      int
	finalErrorMessage     string
	finalErrorCode        string
	history               *responsehistory.Session

	// toolCallRepair / toolCallRepairCtx wire the phase-3 finalize-only LLM
	// tool-call repair into the streaming finalize path (M4). Nil unless the
	// handler enabled repair; the per-chunk sieve never receives them.
	toolCallRepair    toolcall.ToolCallRepairInvoker
	toolCallRepairCtx context.Context
	toolMarker        string
	toolChoice        promptcompat.ToolChoicePolicy
	outputFormat      outputformat.Format
}

func (h *Handler) handleStreamGenerateContentWithRetry(w http.ResponseWriter, r *http.Request, a *auth.RequestAuth, resp *http.Response, payload map[string]any, pow string, stdReq promptcompat.StandardRequest, model, finalPrompt string, thinkingEnabled, searchEnabled bool, toolNames []string, toolsRaw any, historySession *responsehistory.Session) {
	if resp.StatusCode != http.StatusOK {
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		if detail := completionruntime.TryDetectCaptchaFromBody(body); detail != "" {
			config.Logger.Warn("[gemini_stream_retry] captcha challenge detected on initial response", "account", a.AccountID, "detail", detail)
		}
		if historySession != nil {
			historySession.Error(resp.StatusCode, strings.TrimSpace(string(body)), "error", "", "")
		}
		writeGeminiError(w, resp.StatusCode, strings.TrimSpace(string(body)))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	rc := http.NewResponseController(w)
	_, canFlush := w.(http.Flusher)
	// Catalogs and history retain incremental prose delivery; only a required
	// tool policy holds the entire answer until final validation.
	runtime := newGeminiStreamRuntime(w, rc, canFlush, model, finalPrompt, thinkingEnabled, searchEnabled, stripReferenceMarkersEnabled(), toolNames, toolsRaw, stdReq.SystemTextToolsApply(), historySession)
	// M4: wire the phase-3 finalize-only LLM repair into the stream finalize.
	runtime.toolCallRepair = completionruntime.NewToolCallRepairInvoker(h.DS, a, h.ChatHistory, stdReq)
	runtime.toolCallRepairCtx = r.Context()
	runtime.refFileTokens = stdReq.RefFileTokens
	runtime.toolMarker = a.ToolMarker
	runtime.toolSieve.Marker = a.ToolMarker
	runtime.toolChoice = stdReq.ToolChoice
	runtime.outputFormat = stdReq.OutputFormat
	runtime.bufferContent = completionruntime.HoldToolOutput(toolNames, toolsRaw, stdReq.ToolChoice, stdReq.OutputFormat)

	completionruntime.ExecuteStreamWithRetry(r.Context(), h.DS, a, resp, payload, pow, completionruntime.StreamRetryOptions{
		Surface:             "gemini.generate_content",
		Stream:              true,
		RetryEnabled:        true,
		MaxAttempts:         3,
		UsagePrompt:         finalPrompt,
		Request:             stdReq,
		CurrentInputFile:    h.Store,
		ExpertPromptSegment: h.Store,
	}, completionruntime.StreamRetryHooks{
		ResponseCompleted: runtime.output.Completed,
		DiagnosticOutput:  runtime.accumulator.DiagnosticOutput,
		ConsumeAttempt: func(currentResp *http.Response, allowDeferEmpty bool) completionruntime.ConsumeAttemptResult {
			return h.consumeGeminiStreamAttempt(r.Context(), currentResp, runtime, thinkingEnabled, allowDeferEmpty)
		},
		Finalize: func(_ int) {
			runtime.finalize(false)
		},
		ParentMessageID: func() int {
			return runtime.responseMessageID
		},
		// Without this hook an empty-output retry would append the new
		// answer to the previous attempt's raw buffers, and tool-call
		// detection would run over the concatenated text of both rounds
		// (chat and claude already reset; see their resetForRegeneration).
		OnRegenerate:   runtime.resetForRegeneration,
		RejectedOutput: runtime.rejectedOutput,
		OnRequest:      func(request promptcompat.StandardRequest) { runtime.refFileTokens = request.RefFileTokens },
		OnRetryPrompt: func(prompt string) {
			runtime.finalPrompt = prompt
		},
		OnRetryFailure: func(status int, message, code string) {
			runtime.sendErrorChunk(status, strings.TrimSpace(message), code)
		},
		EmptyOutputError: func() *assistantturn.OutputError {
			if runtime.finalErrorCode == "" {
				return nil
			}
			return &assistantturn.OutputError{Status: runtime.finalErrorStatus, Message: runtime.finalErrorMessage, Code: runtime.finalErrorCode}
		},
	})
}

func (h *Handler) consumeGeminiStreamAttempt(ctx context.Context, resp *http.Response, runtime *geminiStreamRuntime, thinkingEnabled bool, allowDeferEmpty bool) completionruntime.ConsumeAttemptResult {
	defer func() { _ = resp.Body.Close() }()
	// Resume keeps delimiter and literal state; regeneration resets it explicitly.
	initialType := runtime.accumulator.InitialType()
	var scannerErr error
	var stopReason streamengine.StopReason
	streamengine.ConsumeSSE(streamengine.ConsumeConfig{
		Context:             ctx,
		Body:                resp.Body,
		ThinkingEnabled:     thinkingEnabled,
		InitialType:         initialType,
		KeepAliveInterval:   time.Duration(dsprotocol.KeepAliveTimeout) * time.Second,
		IdleTimeout:         time.Duration(dsprotocol.StreamIdleTimeout) * time.Second,
		MaxKeepAliveNoInput: dsprotocol.MaxKeepaliveCount,
	}, streamengine.ConsumeHooks{
		OutputError: runtime.output.Err,
		OnParsed:    runtime.onParsed,
		OnFinalize: func(reason streamengine.StopReason, err error) {
			stopReason = reason
			scannerErr = err
		},
		OnContextDone: func() {
			// The client disconnected: close the history entry as "stopped"
			// (with estimated usage) instead of leaving it "streaming" forever.
			// Mirrors the chat surface's OnContextDone handling.
			if runtime.history != nil {
				rawText, text, rawThinking, thinking, detectionThinking := runtime.accumulator.Snapshot()
				runtime.history.StoppedWithUpstream(
					responsehistory.ThinkingForArchive(rawThinking, detectionThinking, thinking),
					responsehistory.TextForArchive(rawText, text),
					string(streamengine.StopReasonContextCancelled), runtime.accumulator.UsageSnapshot(), promptcompat.StandardRequest{PromptTokenText: runtime.finalPrompt, RefFileTokens: runtime.refFileTokens},
				)
			}
		},
	})
	if ctx.Err() != nil {
		// ConsumeSSE does not run OnFinalize when the context is done; without
		// this early return the code below would finalize normally — writing
		// terminal frames to a dead connection and recording the truncated
		// content as a successful 200 in history (chat/claude already bail out).
		return completionruntime.ConsumeAttemptResult{Terminal: true}
	}
	// An upstream stream that died mid-body (read error) or stalled past the
	// idle / no-content window after producing partial output must not be
	// silently finalized as a normal finish frame: schedule a continue-resume so
	// the same message is completed, or surface an explicit interruption error.
	// Mirrors the chat.completions surface (openai/chat/empty_retry_runtime.go).
	interrupted := completionruntime.StreamInterrupted(scannerErr, stopReason)
	if interrupted && runtime.responseMessageID > 0 && runtime.accumulator.HasPartialOutput() {
		config.Logger.Warn("[gemini_stream] upstream stream interrupted; scheduling continue-resume",
			"surface", "gemini.generate_content", "stream", true, "reason", stopReason, "error", fmt.Sprint(scannerErr), "response_message_id", runtime.responseMessageID)
		return completionruntime.ConsumeAttemptResult{Retryable: true, ResumeContinue: true}
	}
	if interrupted && runtime.accumulator.HasPartialOutput() {
		// Partial output but no resumable message id: surface the truncation and
		// close the history entry like the chat surface does, so it does not stay
		// in "streaming" forever.
		runtime.sendErrorChunk(http.StatusBadGateway, "Upstream stream interrupted before completion and cannot be resumed.", "upstream_interrupted")
		if runtime.history != nil {
			rawText, text, rawThinking, thinking, detectionThinking := runtime.accumulator.Snapshot()
			runtime.history.ErrorWithUpstream(http.StatusBadGateway, "Upstream stream interrupted before completion and cannot be resumed.", "upstream_interrupted",
				responsehistory.ThinkingForArchive(rawThinking, detectionThinking, thinking),
				responsehistory.TextForArchive(rawText, text), runtime.accumulator.UsageSnapshot(), promptcompat.StandardRequest{PromptTokenText: runtime.finalPrompt, RefFileTokens: runtime.refFileTokens})
		}
		return completionruntime.ConsumeAttemptResult{Terminal: true}
	}
	terminalWritten := runtime.finalize(allowDeferEmpty)
	if terminalWritten {
		return completionruntime.ConsumeAttemptResult{Terminal: true, Failure: &assistantturn.OutputError{Status: runtime.finalErrorStatus, Code: runtime.finalErrorCode, Message: runtime.finalErrorMessage}}
	}
	return completionruntime.ConsumeAttemptResult{Retryable: true}
}

// resetForRegeneration discards the previous attempt's accumulated output
// before a replacement generation starts (empty-output retry or account
// switch). Delivered tool preambles remain visible, while replacement prose
// waits for validation. Mirrors chat/claude stream_regenerate.go.
func (s *geminiStreamRuntime) resetForRegeneration() {
	s.toolRecovery.Start(s.finalErrorCode, s.answerDelivered)
	s.diagnosticProgress = assistantturn.DiagnosticProgress{}
	s.accumulator.ResetForRegeneration()
	s.toolSieve = toolstream.State{CleanVisible: true, DeferMalformed: true, Marker: s.toolMarker, StripReferenceMarkers: s.stripReferenceMarkers}
	s.deferredText.Reset()
	s.leadingText.Reset()
	s.toolCallsPending = false
	// Delivered prose remains committed across replacement attempts.
	s.contentFilter = false
	s.responseMessageID = 0
	s.upstreamErr = ""
	s.finalErrorStatus, s.finalErrorMessage, s.finalErrorCode = 0, "", ""
}

//nolint:unused // retained for native Gemini stream handling path.
func newGeminiStreamRuntime(
	w http.ResponseWriter,
	rc *http.ResponseController,
	canFlush bool,
	model string,
	finalPrompt string,
	thinkingEnabled bool,
	searchEnabled bool,
	stripReferenceMarkers bool,
	toolNames []string,
	toolsRaw any,
	_ bool, // catalogs do not delay ordinary prose
	history *responsehistory.Session,
) *geminiStreamRuntime {
	return &geminiStreamRuntime{
		w:                     w,
		rc:                    rc,
		canFlush:              canFlush,
		model:                 model,
		finalPrompt:           finalPrompt,
		thinkingEnabled:       thinkingEnabled,
		searchEnabled:         searchEnabled,
		bufferContent:         false,
		toolSieve:             toolstream.State{CleanVisible: true, DeferMalformed: true, StripReferenceMarkers: stripReferenceMarkers},
		stripReferenceMarkers: stripReferenceMarkers,
		toolNames:             toolNames,
		toolsRaw:              toolsRaw,
		history:               history,
		accumulator: assistantturn.NewAccumulator(assistantturn.AccumulatorOptions{
			Prompt:                finalPrompt,
			ThinkingEnabled:       thinkingEnabled,
			SearchEnabled:         searchEnabled,
			StripReferenceMarkers: stripReferenceMarkers,
		}),
	}
}

//nolint:unused // retained for native Gemini stream handling path.
func (s *geminiStreamRuntime) sendChunk(payload map[string]any) {
	s.output.JSON(s.w, s.rc, s.canFlush, "", payload)
}

// countInputTokens memoizes util.CountPromptTokens per prompt text within
// one request.
func (s *geminiStreamRuntime) countInputTokens(prompt string) int {
	if s == nil || prompt == "" {
		return 0
	}
	if s.inputTokenCounts == nil {
		s.inputTokenCounts = make(map[string]int, 2)
	}
	if v, ok := s.inputTokenCounts[prompt]; ok {
		return v
	}
	v := util.CountPromptTokens(prompt, s.model)
	s.inputTokenCounts[prompt] = v
	return v
}

// promptTokenCountFor is the assistantturn.BuildOptions.PromptTokenCountFn
// adapter around countInputTokens.
func (s *geminiStreamRuntime) promptTokenCountFor(prompt string) (int, bool) {
	if s == nil || prompt == "" {
		return 0, false
	}
	return s.countInputTokens(prompt), true
}

func (s *geminiStreamRuntime) sendErrorChunk(status int, message string, codes ...string) {
	msg := strings.TrimSpace(message)
	if msg == "" {
		msg = http.StatusText(status)
	}
	s.finalErrorStatus, s.finalErrorMessage, s.finalErrorCode = status, msg, "upstream_error"
	if len(codes) > 0 && codes[0] != "" {
		s.finalErrorCode = codes[0]
	}
	errorStatus := "INVALID_ARGUMENT"
	switch status {
	case http.StatusUnauthorized:
		errorStatus = "UNAUTHENTICATED"
	case http.StatusForbidden:
		errorStatus = "PERMISSION_DENIED"
	case http.StatusTooManyRequests:
		errorStatus = "RESOURCE_EXHAUSTED"
	case http.StatusNotFound:
		errorStatus = "NOT_FOUND"
	default:
		if status >= 500 {
			errorStatus = "INTERNAL"
		}
	}
	s.sendChunk(map[string]any{
		"error": map[string]any{
			"code":    status,
			"message": msg,
			"status":  errorStatus,
		},
	})
}

//nolint:unused // retained for native Gemini stream handling path.
func (s *geminiStreamRuntime) finalize(deferEmptyOutput bool) bool {
	if s.accumulator.Cut() == sse.CutNone {
		s.onParsed(sse.LineResult{Parsed: true, Flush: true})
	}
	if s.output.Err() != nil {
		s.finalErrorStatus, s.finalErrorCode, s.finalErrorMessage = 499, "client_write_error", s.output.Err().Error()
		return true
	}
	s.finalErrorStatus, s.finalErrorMessage, s.finalErrorCode = 0, "", ""
	rawText, text, rawThinking, thinking, detectionThinking := s.accumulator.Snapshot()
	turn := assistantturn.BuildTurnFromStreamSnapshot(assistantturn.StreamSnapshot{
		UpstreamUsage:     s.accumulator.UsageSnapshot(),
		ContentCut:        s.accumulator.Cut(),
		RawText:           rawText,
		VisibleText:       text,
		RawThinking:       rawThinking,
		VisibleThinking:   thinking,
		DetectionThinking: detectionThinking,
		ContentFilter:     s.contentFilter,
		UpstreamError:     s.upstreamErr,
		ResponseMessageID: s.responseMessageID,
	}, assistantturn.BuildOptions{
		Model:                 s.model,
		Prompt:                s.finalPrompt,
		RefFileTokens:         s.refFileTokens,
		PromptTokenCountFn:    s.promptTokenCountFor,
		SearchEnabled:         s.searchEnabled,
		StripReferenceMarkers: s.stripReferenceMarkers,
		ToolNames:             s.toolNames,
		ToolsRaw:              s.toolsRaw,
		ToolChoice:            s.toolChoice,
		OutputFormat:          s.outputFormat,
		ToolCallRepair:        s.toolCallRepair,
		ToolCallRepairCtx:     s.toolCallRepairCtx,
		ToolMarker:            s.toolMarker,
	})
	outcome := assistantturn.FinalizeTurn(turn, assistantturn.FinalizeOptions{})
	if outcome.ShouldFail {
		if completionruntime.CanDeferToolAwareOutputError(outcome.Error, deferEmptyOutput, s.answerDelivered, s.toolsDelivered, s.toolCallRepairCtx) {
			s.finalErrorStatus = outcome.Error.Status
			s.finalErrorMessage = outcome.Error.Message
			s.finalErrorCode = outcome.Error.Code
			return false
		}
		if s.history != nil {
			s.history.ErrorWithUpstream(outcome.Error.Status, outcome.Error.Message, outcome.Error.Code, responsehistory.ThinkingForArchive(turn.RawThinking, turn.DetectionThinking, turn.Thinking), responsehistory.TextForArchive(turn.RawText, turn.Text), s.accumulator.UsageSnapshot(), promptcompat.StandardRequest{PromptTokenText: s.finalPrompt, RefFileTokens: s.refFileTokens})
		}
		s.sendErrorChunk(outcome.Error.Status, outcome.Error.Message, outcome.Error.Code)
		return true
	}

	turn.Thinking = "" // Visible thinking has already streamed on every path.
	if s.bufferContent || s.toolRecovery.Hold {
		s.emitStreamText(s.toolRecovery.Text(turn, s.toolNames, s.toolMarker, s.stripReferenceMarkers))
	}
	if len(turn.ToolCalls) > 0 {
		turn.Text = "" // The prefix was delivered incrementally or flushed above.
		s.sendParts(buildGeminiPartsFromTurn(turn))
		if s.output.ToolsWritten(s.toolCallRepairCtx) {
			s.answerDelivered = true
			s.toolsDelivered = true
		}
	} else if !s.bufferContent && !s.toolRecovery.Hold {
		s.emitStreamText(toolstream.FilterVisible(s.deferredText.String(), ""))
		s.deferredText.Reset()
		for _, event := range toolstream.Flush(&s.toolSieve, s.toolNames) {
			text := event.Content
			if event.Deferred {
				text = toolstream.FilterVisible(text, "")
			}
			s.emitStreamText(text)
		}
	}

	s.sendChunk(map[string]any{
		"candidates": []map[string]any{
			{
				"index": 0,
				"content": map[string]any{
					"role": "model",
					"parts": []map[string]any{
						{"text": ""},
					},
				},
				"finishReason": "STOP",
			},
		},
		"modelVersion":  s.model,
		"usageMetadata": assistantturn.GeminiUsage(turn),
	})
	if !s.output.Complete(s.toolCallRepairCtx) {
		s.finalErrorStatus, s.finalErrorCode, s.finalErrorMessage = 499, "client_write_error", s.output.Err().Error()
		if s.history != nil {
			s.history.ErrorWithUpstream(499, s.finalErrorMessage, s.finalErrorCode, responsehistory.ThinkingForArchive(turn.RawThinking, turn.DetectionThinking, turn.Thinking), responsehistory.TextForArchive(turn.RawText, turn.Text), s.accumulator.UsageSnapshot(), promptcompat.StandardRequest{PromptTokenText: s.finalPrompt, RefFileTokens: s.refFileTokens})
		}
	} else if s.history != nil {
		s.history.Success(
			http.StatusOK,
			responsehistory.ThinkingForArchive(turn.RawThinking, turn.DetectionThinking, turn.Thinking),
			responsehistory.TextForArchive(turn.RawText, turn.Text),
			assistantturn.FinishReason(turn),
			responsehistory.GenericUsage(turn),
		)
	}
	return true
}
