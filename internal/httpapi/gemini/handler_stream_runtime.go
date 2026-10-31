package gemini

import (
	"context"
	"encoding/json"
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
	"ds2api/internal/promptcompat"
	"ds2api/internal/responsehistory"
	"ds2api/internal/sse"
	streamengine "ds2api/internal/stream"
	"ds2api/internal/toolcall"
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
	// This entry point has no normalized request to ask about the system-text
	// tool branch, so it keeps the tools-array gate; the production path is
	// handleStreamGenerateContentWithRetry, which passes the real answer.
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
		OnParsed: runtime.onParsed,
		OnFinalize: func(_ streamengine.StopReason, _ error) {
			runtime.finalize(false)
		},
	})
}

//nolint:unused // retained for native Gemini stream handling path.
type geminiStreamRuntime struct {
	w        http.ResponseWriter
	rc       *http.ResponseController
	canFlush bool

	model       string
	finalPrompt string
	// inputTokenCounts memoizes the prompt-side BPE count per prompt text so
	// finalize (and each retry attempt's finalize) does not re-encode the same
	// hundreds-of-KB prompt.
	inputTokenCounts map[string]int

	thinkingEnabled bool
	searchEnabled   bool
	// bufferContent is the sieve gate: while it is set, per-chunk text is not
	// forwarded and the whole answer is emitted once from the finalized turn, so
	// tool-call markup is parsed into functionCall parts instead of leaking into
	// the visible text. It follows the prompt — a request whose prompt may demand
	// tool calls (a top-level tools array, or the system-text branch) is sieved;
	// see newGeminiStreamRuntime.
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
	// The sieve gate follows the prompt: a request whose prompt may demand tool
	// calls must have its tool-call markup sieved, whatever shape carried the
	// catalog (see newGeminiStreamRuntime).
	runtime := newGeminiStreamRuntime(w, rc, canFlush, model, finalPrompt, thinkingEnabled, searchEnabled, stripReferenceMarkersEnabled(), toolNames, toolsRaw, stdReq.SystemTextToolsApply(), historySession)
	// M4: wire the phase-3 finalize-only LLM repair into the stream finalize.
	runtime.toolCallRepair = completionruntime.NewToolCallRepairInvoker(h.DS, a, h.ChatHistory)
	runtime.toolCallRepairCtx = r.Context()
	runtime.toolMarker = a.ToolMarker
	runtime.toolChoice = stdReq.ToolChoice
	runtime.bufferContent = runtime.bufferContent || stdReq.ToolChoice.Constrained()

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
		ConsumeAttempt: func(currentResp *http.Response, allowDeferEmpty bool) completionruntime.ConsumeAttemptResult {
			return h.consumeGeminiStreamAttempt(r.Context(), currentResp, runtime, thinkingEnabled, allowDeferEmpty)
		},
		Finalize: func(_ int) {
			runtime.finalize(false)
		},
		ParentMessageID: func() int {
			return runtime.responseMessageID
		},
		OnRetryPrompt: func(prompt string) {
			runtime.finalPrompt = prompt
		},
		OnRetryFailure: func(status int, message, _ string) {
			runtime.sendErrorChunk(status, strings.TrimSpace(message))
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
	// A new attempt reads a new upstream response, so the content rules start
	// over: a cut that ended the previous attempt (an empty-output retry, or a
	// retry on another account) must not keep discarding this one's text.
	runtime.accumulator.ResetContentPolicy()
	initialType := "text"
	if thinkingEnabled {
		initialType = "thinking"
	}
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
		OnParsed: runtime.onParsed,
		OnFinalize: func(reason streamengine.StopReason, err error) {
			stopReason = reason
			scannerErr = err
		},
	})
	// An upstream stream that died mid-body (read error) or stalled past the
	// idle / no-content window after producing partial output must not be
	// silently finalized as a normal finish frame: schedule a continue-resume so
	// the same message is completed, or surface an explicit interruption error.
	// Mirrors the chat.completions surface (openai/chat/empty_retry_runtime.go).
	interrupted := scannerErr != nil || stopReason == streamengine.StopReasonIdleTimeout || stopReason == streamengine.StopReasonNoContentTimeout
	if interrupted && runtime.responseMessageID > 0 && runtime.accumulator.HasPartialOutput() {
		config.Logger.Warn("[gemini_stream] upstream stream interrupted; scheduling continue-resume",
			"surface", "gemini.generate_content", "stream", true, "reason", stopReason, "error", fmt.Sprint(scannerErr), "response_message_id", runtime.responseMessageID)
		return completionruntime.ConsumeAttemptResult{Retryable: true, ResumeContinue: true}
	}
	if interrupted && runtime.accumulator.HasPartialOutput() {
		// Partial output but no resumable message id: surface the truncation and
		// close the history entry like the chat surface does, so it does not stay
		// in "streaming" forever.
		runtime.sendErrorChunk(http.StatusBadGateway, "Upstream stream interrupted before completion and cannot be resumed.")
		if runtime.history != nil {
			rawText, text, rawThinking, thinking, detectionThinking := runtime.accumulator.Snapshot()
			runtime.history.Error(http.StatusBadGateway, "Upstream stream interrupted before completion and cannot be resumed.", "upstream_interrupted",
				responsehistory.ThinkingForArchive(rawThinking, detectionThinking, thinking),
				responsehistory.TextForArchive(rawText, text))
		}
		return completionruntime.ConsumeAttemptResult{Terminal: true}
	}
	terminalWritten := runtime.finalize(allowDeferEmpty)
	if terminalWritten {
		return completionruntime.ConsumeAttemptResult{Terminal: true}
	}
	return completionruntime.ConsumeAttemptResult{Retryable: true}
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
	systemTextTools bool,
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
		bufferContent:         len(toolNames) > 0 || systemTextTools,
		stripReferenceMarkers: stripReferenceMarkers,
		toolNames:             toolNames,
		toolsRaw:              toolsRaw,
		history:               history,
		accumulator: assistantturn.NewAccumulator(assistantturn.AccumulatorOptions{
			ThinkingEnabled:       thinkingEnabled,
			SearchEnabled:         searchEnabled,
			StripReferenceMarkers: stripReferenceMarkers,
		}),
	}
}

//nolint:unused // retained for native Gemini stream handling path.
func (s *geminiStreamRuntime) sendChunk(payload map[string]any) {
	b, _ := json.Marshal(payload)
	_, _ = s.w.Write([]byte("data: "))
	_, _ = s.w.Write(b)
	_, _ = s.w.Write([]byte("\n\n"))
	if s.canFlush {
		_ = s.rc.Flush()
	}
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

func (s *geminiStreamRuntime) sendErrorChunk(status int, message string) {
	msg := strings.TrimSpace(message)
	if msg == "" {
		msg = http.StatusText(status)
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
func (s *geminiStreamRuntime) onParsed(parsed sse.LineResult) streamengine.ParsedDecision {
	if !parsed.Parsed {
		return streamengine.ParsedDecision{}
	}
	if parsed.ResponseMessageID > 0 {
		s.responseMessageID = parsed.ResponseMessageID
	}
	if parsed.ContentFilter || parsed.ErrorMessage != "" || parsed.Stop {
		if parsed.ContentFilter {
			s.contentFilter = true
		}
		if parsed.ErrorMessage != "" {
			s.upstreamErr = parsed.ErrorMessage
		}
		return streamengine.ParsedDecision{Stop: true}
	}

	accumulated := s.accumulator.Apply(parsed)
	for _, p := range accumulated.Parts {
		if p.Type == "thinking" {
			if p.VisibleText == "" || s.bufferContent {
				continue
			}
			s.sendChunk(map[string]any{
				"candidates": []map[string]any{
					{
						"index": 0,
						"content": map[string]any{
							"role":  "model",
							"parts": []map[string]any{{"text": p.VisibleText, "thought": true}},
						},
					},
				},
				"modelVersion": s.model,
			})
			continue
		}
		if p.RawText == "" || p.CitationOnly || p.VisibleText == "" {
			continue
		}
		if s.bufferContent {
			continue
		}
		s.sendChunk(map[string]any{
			"candidates": []map[string]any{
				{
					"index": 0,
					"content": map[string]any{
						"role":  "model",
						"parts": []map[string]any{{"text": p.VisibleText}},
					},
				},
			},
			"modelVersion": s.model,
		})
	}
	if s.history != nil {
		rawText, text, rawThinking, thinking, detectionThinking := s.accumulator.Snapshot()
		s.history.Progress(
			responsehistory.ThinkingForArchive(rawThinking, detectionThinking, thinking),
			responsehistory.TextForArchive(rawText, text),
		)
	}
	// 上游内容规则（复读死循环 / 编造的 `[Tool]:` 角色标记）在此统一收口：
	// 结束读取，保留已生成内容走正常收尾。
	return streamengine.StopForCut(accumulated.Cut, "gemini_stream", "gemini", accumulated.ContentSeen)
}

//nolint:unused // retained for native Gemini stream handling path.
func (s *geminiStreamRuntime) finalize(deferEmptyOutput bool) bool {
	rawText, text, rawThinking, thinking, detectionThinking := s.accumulator.Snapshot()
	turn := assistantturn.BuildTurnFromStreamSnapshot(assistantturn.StreamSnapshot{
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
		PromptTokenCountFn:    s.promptTokenCountFor,
		SearchEnabled:         s.searchEnabled,
		StripReferenceMarkers: s.stripReferenceMarkers,
		ToolNames:             s.toolNames,
		ToolsRaw:              s.toolsRaw,
		ToolChoice:            s.toolChoice,
		ToolCallRepair:        s.toolCallRepair,
		ToolCallRepairCtx:     s.toolCallRepairCtx,
		ToolMarker:            s.toolMarker,
	})
	outcome := assistantturn.FinalizeTurn(turn, assistantturn.FinalizeOptions{})
	if outcome.ShouldFail {
		if deferEmptyOutput {
			s.finalErrorStatus = outcome.Error.Status
			s.finalErrorMessage = outcome.Error.Message
			s.finalErrorCode = outcome.Error.Code
			return false
		}
		if s.history != nil {
			s.history.Error(outcome.Error.Status, outcome.Error.Message, outcome.Error.Code, responsehistory.ThinkingForArchive(turn.RawThinking, turn.DetectionThinking, turn.Thinking), responsehistory.TextForArchive(turn.RawText, turn.Text))
		}
		s.sendErrorChunk(outcome.Error.Status, outcome.Error.Message)
		return true
	}
	if s.history != nil {
		s.history.Success(
			http.StatusOK,
			responsehistory.ThinkingForArchive(turn.RawThinking, turn.DetectionThinking, turn.Thinking),
			responsehistory.TextForArchive(turn.RawText, turn.Text),
			assistantturn.FinishReason(turn),
			responsehistory.GenericUsage(turn),
		)
	}

	if s.bufferContent {
		parts := buildGeminiPartsFromTurn(turn)
		s.sendChunk(map[string]any{
			"candidates": []map[string]any{
				{
					"index": 0,
					"content": map[string]any{
						"role":  "model",
						"parts": parts,
					},
				},
			},
			"modelVersion": s.model,
		})
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
		"modelVersion": s.model,
		"usageMetadata": map[string]any{
			"promptTokenCount":     outcome.Usage.InputTokens,
			"candidatesTokenCount": outcome.Usage.OutputTokens,
			"totalTokenCount":      outcome.Usage.TotalTokens,
		},
	})
	return true
}
