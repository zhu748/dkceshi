package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"

	"ds2api/internal/assistantturn"
	"ds2api/internal/auth"
	"ds2api/internal/completionruntime"
	"ds2api/internal/config"
	"ds2api/internal/contextfile"
	claudefmt "ds2api/internal/format/claude"
	"ds2api/internal/httpapi/openai/files"
	"ds2api/internal/httpapi/openai/shared"
	"ds2api/internal/httpapi/requestbody"
	"ds2api/internal/jsonvalue"
	"ds2api/internal/promptcompat"
	"ds2api/internal/responsehistory"
	streamengine "ds2api/internal/stream"
	"ds2api/internal/translatorcliproxy"
	"ds2api/internal/util"
)

func (h *Handler) Messages(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(r.Header.Get("anthropic-version")) == "" {
		r.Header.Set("anthropic-version", "2023-06-01")
	}
	if isClaudeVercelProxyRequest(r) && h.proxyViaOpenAI(w, r, h.Store) {
		return
	}
	if h.Auth == nil || h.DS == nil {
		if h.OpenAI != nil && h.proxyViaOpenAI(w, r, h.Store) {
			return
		}
		writeClaudeError(w, http.StatusInternalServerError, "Claude runtime backend unavailable.")
		return
	}
	if h.handleClaudeDirect(w, r) {
		return
	}
	writeClaudeError(w, http.StatusBadGateway, "Failed to handle Claude request.")
}

func isClaudeVercelProxyRequest(r *http.Request) bool {
	if r == nil || r.URL == nil {
		return false
	}
	return strings.TrimSpace(r.URL.Query().Get("__stream_prepare")) == "1" ||
		strings.TrimSpace(r.URL.Query().Get("__stream_release")) == "1"
}

func (h *Handler) handleClaudeDirect(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, shared.GeneralMaxSize)
	// Decode straight from the replay body: the UTF-8/JSON validator already
	// buffers the payload once, and io.ReadAll here made multimodal requests
	// hold two full copies (plus the decoded map).
	var req map[string]any
	if err := jsonvalue.Decode(r.Body, &req); err != nil {
		switch {
		case requestbody.IsTooLarge(err):
			writeClaudeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		case errors.Is(err, requestbody.ErrInvalidUTF8Body), errors.Is(err, requestbody.ErrInvalidJSONBody):
			writeClaudeError(w, http.StatusBadRequest, "invalid json")
		default:
			writeClaudeError(w, http.StatusBadRequest, "invalid body")
		}
		return true
	}
	// 是否注入工具提示词与号池路由均按“body 是否携带工具定义”决定，写入上下文供 Determine 读取。
	// 按 Key 的开关由 promptcompat.ApplyCallerFlags 在判定之前应用。
	r = r.WithContext(auth.WithToolsPresent(r.Context(), promptcompat.ApplyCallerFlags(h.Auth, r, req)))
	norm, err := normalizeClaudeRequest(h.Store, req)
	if err != nil {
		writeClaudeError(w, http.StatusBadRequest, err.Error())
		return true
	}
	exposeThinking := norm.Standard.Thinking
	a, err := h.Auth.Determine(r)
	if err != nil {
		writeClaudeError(w, auth.RequestErrorStatus(err), err.Error())
		return true
	}
	defer h.Auth.Release(a)
	if err := files.PreprocessInlineFiles(r.Context(), a, req, h.DS, norm.Standard.ResolvedModel, norm.Standard.Thinking); err != nil {
		status, message := files.InlineFileErrorDetails(err)
		writeClaudeError(w, status, message)
		return true
	}
	norm, err = normalizeClaudeRequest(h.Store, req)
	if err != nil {
		writeClaudeError(w, http.StatusBadRequest, err.Error())
		return true
	}
	norm.Standard.ToolMarker = a.ToolMarker
	norm.Standard.FinalPrompt, norm.Standard.ToolNames = norm.Standard.BuildPromptFromMessages(norm.Standard.Messages)
	// Usage 口径必须等于实际发送的那份 prompt：OpenAI 出口在
	// NormalizeOpenAIChatRequest / NormalizeOpenAIResponsesRequest 内把
	// PromptTokenText 与出口替换后的 FinalPrompt 一起赋值，这里同步一次，
	// 否则同一个设置（思考注入 / input_file 续写）开与不开会让 Claude 的
	// input_tokens 分别按两种形态计数。
	norm.Standard.PromptTokenText = norm.Standard.FinalPrompt
	stdReq, err := h.applyCurrentInputFile(r.Context(), a, norm.Standard)
	if err != nil {
		status, message := mapCurrentInputFileError(err)
		writeClaudeError(w, status, message)
		return true
	}
	historySession := responsehistory.Start(responsehistory.StartParams{
		Usage:    h.Usage,
		Store:    h.ChatHistory,
		Request:  r,
		Auth:     a,
		Surface:  "claude.messages",
		Standard: stdReq,
	})
	if stdReq.Stream {
		h.handleClaudeDirectStream(w, r, a, stdReq, historySession)
		return true
	}
	result, outErr := completionruntime.ExecuteNonStreamWithRetry(r.Context(), h.DS, a, stdReq, completionruntime.Options{
		RetryEnabled:          true,
		CurrentInputFile:      h.Store,
		ExpertPromptSegment:   h.Store,
		ToolCallRepairEnabled: true,
		ChatHistory:           h.ChatHistory,
	})
	if outErr != nil {
		if historySession != nil {
			if completionruntime.IsDirectTokenAuthError(outErr) {
				historySession.Discard()
			} else {
				historySession.ErrorTurn(outErr.Status, outErr.Message, outErr.Code, result.Turn)
			}
		}
		writeClaudeError(w, outErr.Status, outErr.Message)
		return true
	}
	if historySession != nil {
		historySession.SuccessTurn(http.StatusOK, result.Turn, responsehistory.GenericUsage(result.Turn))
	}
	writeJSON(w, http.StatusOK, claudefmt.BuildMessageResponseFromTurn(
		fmt.Sprintf("msg_%d", time.Now().UnixNano()),
		stdReq.ResponseModel,
		result.Turn,
		exposeThinking,
	))
	return true
}

func (h *Handler) applyCurrentInputFile(ctx context.Context, a *auth.RequestAuth, stdReq promptcompat.StandardRequest) (promptcompat.StandardRequest, error) {
	if h == nil {
		return stdReq, nil
	}
	stdReq = promptcompat.ApplyThinkingInjection(h.Store, stdReq)
	return (contextfile.Service{Store: h.Store, DS: h.DS}).ApplyCurrentInputFile(ctx, a, stdReq)
}

func mapCurrentInputFileError(err error) (int, string) {
	return contextfile.MapError(err)
}

func (h *Handler) handleClaudeDirectStream(w http.ResponseWriter, r *http.Request, a *auth.RequestAuth, stdReq promptcompat.StandardRequest, historySession *responsehistory.Session) {
	start, outErr := completionruntime.StartCompletion(r.Context(), h.DS, a, stdReq, completionruntime.Options{
		CurrentInputFile:    h.Store,
		ExpertPromptSegment: h.Store,
	})
	if outErr != nil {
		if historySession != nil {
			if completionruntime.IsDirectTokenAuthError(outErr) {
				historySession.Discard()
			} else {
				historySession.Error(outErr.Status, outErr.Message, outErr.Code, "", "")
			}
		}
		writeClaudeError(w, outErr.Status, outErr.Message)
		return
	}
	streamReq := start.Request
	h.handleClaudeStreamRealtimeWithRetry(w, r, a, start.Response, start.Payload, start.Pow, streamReq, streamReq.ResponseModel, streamReq.Messages, streamReq.Thinking, streamReq.Search, streamReq.ToolNames, streamReq.ToolsRaw, streamReq.PromptTokenText, historySession)
}

func (h *Handler) proxyViaOpenAI(w http.ResponseWriter, r *http.Request, store ConfigReader) bool {
	r.Body = http.MaxBytesReader(w, r.Body, shared.GeneralMaxSize)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		if requestbody.IsTooLarge(err) {
			writeClaudeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		} else if errors.Is(err, requestbody.ErrInvalidUTF8Body) {
			writeClaudeError(w, http.StatusBadRequest, "invalid json")
		} else {
			writeClaudeError(w, http.StatusBadRequest, "invalid body")
		}
		return true
	}
	var req map[string]any
	if err := jsonvalue.Unmarshal(raw, &req); err != nil {
		writeClaudeError(w, http.StatusBadRequest, "invalid json")
		return true
	}
	model, _ := req["model"].(string)
	stream := util.ToBool(req["stream"])

	// Use the shared global model resolver so Claude/OpenAI/Gemini stay consistent.
	translateModel := model
	if store != nil {
		if norm, normErr := normalizeClaudeRequest(store, cloneMap(req)); normErr == nil && strings.TrimSpace(norm.Standard.ResolvedModel) != "" {
			translateModel = strings.TrimSpace(norm.Standard.ResolvedModel)
		}
	}
	translatedReq := translatorcliproxy.ToOpenAI(sdktranslator.FormatClaude, translateModel, raw, stream)
	translatedReq, exposeThinking := applyClaudeThinkingPolicyToOpenAIRequest(translatedReq, req)

	isVercelPrepare := strings.TrimSpace(r.URL.Query().Get("__stream_prepare")) == "1"
	isVercelRelease := strings.TrimSpace(r.URL.Query().Get("__stream_release")) == "1"

	if isVercelRelease {
		proxyReq := r.Clone(r.Context())
		proxyReq.URL.Path = "/v1/chat/completions"
		proxyReq.Body = io.NopCloser(bytes.NewReader(raw))
		proxyReq.ContentLength = int64(len(raw))
		rec := httptest.NewRecorder()
		h.OpenAI.ChatCompletions(rec, proxyReq)
		res := rec.Result()
		defer func() { _ = res.Body.Close() }()
		body, _ := io.ReadAll(res.Body)
		for k, vv := range res.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(res.StatusCode)
		_, _ = w.Write(body)
		return true
	}

	proxyReq := r.Clone(r.Context())
	proxyReq.URL.Path = "/v1/chat/completions"
	proxyReq.Body = io.NopCloser(bytes.NewReader(translatedReq))
	proxyReq.ContentLength = int64(len(translatedReq))

	if stream && !isVercelPrepare {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache, no-transform")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		streamWriter := translatorcliproxy.NewOpenAIStreamTranslatorWriter(w, sdktranslator.FormatClaude, model, raw, translatedReq)
		h.OpenAI.ChatCompletions(streamWriter, proxyReq)
		return true
	}

	rec := httptest.NewRecorder()
	h.OpenAI.ChatCompletions(rec, proxyReq)
	res := rec.Result()
	defer func() { _ = res.Body.Close() }()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		for k, vv := range res.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(res.StatusCode)
		_, _ = w.Write(body)
		return true
	}
	if isVercelPrepare {
		for k, vv := range res.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(res.StatusCode)
		_, _ = w.Write(body)
		return true
	}
	converted := translatorcliproxy.FromOpenAINonStream(sdktranslator.FormatClaude, model, raw, translatedReq, body)
	if !exposeThinking {
		converted = stripClaudeThinkingBlocks(converted)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(converted)
	return true
}

func applyClaudeThinkingPolicyToOpenAIRequest(translated []byte, original map[string]any) ([]byte, bool) {
	req := map[string]any{}
	if err := jsonvalue.Unmarshal(translated, &req); err != nil {
		return translated, false
	}
	enabled, ok := util.ResolveThinkingOverride(original)
	if !ok {
		if _, translatedHasOverride := util.ResolveThinkingOverride(req); translatedHasOverride {
			return translated, false
		}
		enabled = true
	}
	typ := "disabled"
	if enabled {
		typ = "enabled"
	}
	req["thinking"] = map[string]any{"type": typ}
	out, err := json.Marshal(req)
	if err != nil {
		return translated, enabled
	}
	return out, enabled
}

func stripClaudeThinkingBlocks(raw []byte) []byte {
	var payload map[string]any
	if err := jsonvalue.Unmarshal(raw, &payload); err != nil {
		return raw
	}
	content, _ := payload["content"].([]any)
	if len(content) == 0 {
		return raw
	}
	filtered := make([]any, 0, len(content))
	for _, item := range content {
		block, _ := item.(map[string]any)
		blockType, _ := block["type"].(string)
		if strings.TrimSpace(blockType) == "thinking" {
			continue
		}
		filtered = append(filtered, item)
	}
	payload["content"] = filtered
	out, err := json.Marshal(payload)
	if err != nil {
		return raw
	}
	return out
}

func (h *Handler) handleClaudeStreamRealtime(w http.ResponseWriter, r *http.Request, resp *http.Response, model string, messages []any, thinkingEnabled, searchEnabled bool, toolNames []string, toolsRaw any, historySessions ...*responsehistory.Session) {
	var historySession *responsehistory.Session
	if len(historySessions) > 0 {
		historySession = historySessions[0]
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if detail := completionruntime.TryDetectCaptchaFromBody(body); detail != "" {
			config.Logger.Warn("[claude_stream] captcha challenge detected on initial response", "detail", detail)
		}
		if historySession != nil {
			historySession.Error(resp.StatusCode, strings.TrimSpace(string(body)), "error", "", "")
		}
		// Preserve the upstream status: writeClaudeError already maps it to the
		// Anthropic error type (429 -> rate_limit_error, 5xx -> api_error, ...).
		// Rewriting everything to 500 made clients treat upstream 429 as a
		// permanent server error instead of retryable rate limiting, and
		// diverged from the status recorded in history above.
		writeClaudeError(w, resp.StatusCode, strings.TrimSpace(string(body)))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	rc := http.NewResponseController(w)
	_, canFlush := w.(http.Flusher)
	if !canFlush {
		config.Logger.Warn("[claude_stream] response writer does not support flush; streaming may be buffered")
	}

	streamRuntime := newClaudeStreamRuntime(
		w,
		rc,
		canFlush,
		model,
		messages,
		thinkingEnabled,
		searchEnabled,
		stripReferenceMarkersEnabled(),
		toolNames,
		toolsRaw,
		promptcompat.RenderPromptText(messages, thinkingEnabled),
		historySession,
	)
	streamRuntime.sendMessageStart()

	initialType := "text"
	if thinkingEnabled {
		initialType = "thinking"
	}
	streamengine.ConsumeSSE(streamengine.ConsumeConfig{
		Context:             r.Context(),
		Body:                resp.Body,
		ThinkingEnabled:     thinkingEnabled,
		InitialType:         initialType,
		KeepAliveInterval:   claudeStreamPingInterval,
		IdleTimeout:         claudeStreamIdleTimeout,
		MaxKeepAliveNoInput: claudeStreamMaxKeepaliveCnt,
	}, streamengine.ConsumeHooks{
		OnKeepAlive: func() {
			streamRuntime.sendPing()
		},
		OnParsed:   streamRuntime.onParsed,
		OnFinalize: streamRuntime.onFinalize,
	})
}

func (h *Handler) handleClaudeStreamRealtimeWithRetry(w http.ResponseWriter, r *http.Request, a *auth.RequestAuth, resp *http.Response, payload map[string]any, pow string, stdReq promptcompat.StandardRequest, model string, messages []any, thinkingEnabled, searchEnabled bool, toolNames []string, toolsRaw any, promptTokenText string, historySession *responsehistory.Session) {
	if resp.StatusCode != http.StatusOK {
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		if detail := completionruntime.TryDetectCaptchaFromBody(body); detail != "" {
			config.Logger.Warn("[claude_stream_realtime] captcha challenge detected on initial response", "account", a.AccountID, "detail", detail)
		}
		if historySession != nil {
			historySession.Error(resp.StatusCode, strings.TrimSpace(string(body)), "error", "", "")
		}
		// Preserve the upstream status (see handleClaudeStreamRealtime for the
		// rationale): writeClaudeError maps 429 -> rate_limit_error etc.
		writeClaudeError(w, resp.StatusCode, strings.TrimSpace(string(body)))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	rc := http.NewResponseController(w)
	_, canFlush := w.(http.Flusher)
	if !canFlush {
		config.Logger.Warn("[claude_stream] response writer does not support flush; streaming may be buffered")
	}

	streamRuntime := newClaudeStreamRuntime(
		w,
		rc,
		canFlush,
		model,
		messages,
		thinkingEnabled,
		searchEnabled,
		stripReferenceMarkersEnabled(),
		toolNames,
		toolsRaw,
		promptTokenText,
		historySession,
	)
	streamRuntime.toolChoice = stdReq.ToolChoice
	streamRuntime.sieve.Marker = a.ToolMarker
	// M4: wire the phase-3 finalize-only LLM repair into the stream finalize.
	streamRuntime.toolCallRepair = completionruntime.NewToolCallRepairInvoker(h.DS, a, h.ChatHistory)
	streamRuntime.toolCallRepairCtx = r.Context()
	streamRuntime.sendMessageStart()

	completionruntime.ExecuteStreamWithRetry(r.Context(), h.DS, a, resp, payload, pow, completionruntime.StreamRetryOptions{
		Surface:             "claude.messages",
		Stream:              true,
		RetryEnabled:        true,
		MaxAttempts:         3,
		UsagePrompt:         promptTokenText,
		Request:             stdReq,
		CurrentInputFile:    h.Store,
		ExpertPromptSegment: h.Store,
	}, completionruntime.StreamRetryHooks{
		ConsumeAttempt: func(currentResp *http.Response, allowDeferEmpty bool) completionruntime.ConsumeAttemptResult {
			return h.consumeClaudeStreamAttempt(r, currentResp, streamRuntime, thinkingEnabled, allowDeferEmpty)
		},
		Finalize: func(_ int) {
			streamRuntime.finalize("end_turn", false)
		},
		ParentMessageID: func() int {
			return streamRuntime.responseMessageID
		},
		OnRegenerate: streamRuntime.resetForRegeneration,
		OnRetryPrompt: func(prompt string) {
			streamRuntime.promptTokenText = prompt
		},
		OnRetryFailure: func(status int, message, code string) {
			streamRuntime.failRetry(status, strings.TrimSpace(message), code)
		},
		EmptyOutputError: func() *assistantturn.OutputError {
			if streamRuntime.finalErrorCode == "" {
				return nil
			}
			return &assistantturn.OutputError{Status: streamRuntime.finalErrorStatus, Message: streamRuntime.finalErrorMessage, Code: streamRuntime.finalErrorCode}
		},
	})
}
