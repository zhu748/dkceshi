package chat

import (
	"ds2api/internal/jsonvalue"
	"io"
	"net/http"
	"strings"
	"time"

	"ds2api/internal/assistantturn"
	"ds2api/internal/auth"
	"ds2api/internal/completionruntime"
	"ds2api/internal/config"
	dsprotocol "ds2api/internal/deepseek/protocol"
	openaifmt "ds2api/internal/format/openai"
	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
	streamengine "ds2api/internal/stream"
)

func (h *Handler) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	if r.URL.Query().Get("__stream_progress") == "1" {
		h.handleVercelStreamProgress(w, r)
		return
	}
	if r.URL.Query().Get("__stream_validate") == "1" {
		h.handleVercelStreamValidate(w, r)
		return
	}
	if isVercelStreamReleaseRequest(r) {
		h.handleVercelStreamRelease(w, r)
		return
	}
	if isVercelStreamPowRequest(r) {
		h.handleVercelStreamPow(w, r)
		return
	}
	if isVercelStreamSwitchRequest(r) {
		h.handleVercelStreamSwitch(w, r)
		return
	}
	if isVercelStreamPrepareRequest(r) {
		h.handleVercelStreamPrepare(w, r)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, openAIGeneralMaxSize)
	var req map[string]any
	if err := jsonvalue.Decode(r.Body, &req); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "too large") {
			writeOpenAIError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		writeOpenAIError(w, http.StatusBadRequest, "invalid json")
		return
	}
	promptcompat.CaptureInputFieldDigests(req)
	// 是否注入工具提示词与号池路由均按“请求 body 是否携带工具定义”决定，
	// 写入请求上下文供 Determine 使用。按 Key 的开关（如「强制禁用工具调用」）
	// 由 promptcompat.ApplyCallerFlags 在判定之前应用，这里不再逐个开关接线。
	r = r.WithContext(auth.WithToolsPresent(r.Context(), promptcompat.ApplyCallerFlags(h.Auth, r, req, "openai_chat")))

	a, err := h.Auth.Determine(r)
	if err != nil {
		status := auth.RequestErrorStatus(err)
		detail := err.Error()
		writeOpenAIError(w, status, detail)
		return
	}
	var sessionID string
	defer func() {
		if !a.FinishAccountResources() {
			h.autoDeleteRemoteSession(r.Context(), a, sessionID)
		}
		h.Auth.Release(a)
	}()

	r = r.WithContext(auth.WithAuth(r.Context(), a))

	// 图片处理分三步且顺序固定：先判定（此时图片块还在，上传要用），再在两轮
	// 内联上传之后剥离图片块（图片改经 ref_file_ids 送达），最后在标准化之后
	// 还原客户端请求的模型名。
	imageRoute := promptcompat.PlanImageRoute(req, h.Store)
	if err := h.preprocessInlineFileInputs(r.Context(), a, req); err != nil {
		h.writeInlinePreparationError(w, r, a, req, err, startedAt)
		return
	}
	if err := h.preprocessInlineTextFilesForExpert(r.Context(), a, req); err != nil {
		h.writeInlinePreparationError(w, r, a, req, err, startedAt)
		return
	}
	imageRoute.StripImages(req)
	stdReq, err := promptcompat.NormalizeOpenAIChatRequest(h.Store, req, requestTraceID(r), a.ToolMarker)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, err.Error())
		return
	}
	stdReq = h.withInputReplay(stdReq, a, req, requestTraceID(r), imageRoute)
	stdReq, err = h.applyCurrentInputFile(r.Context(), a, stdReq)
	if err != nil {
		h.writeContextPreparationError(w, r, a, stdReq, err, startedAt)
		return
	}
	historySession := startChatHistoryWithUsage(h.ChatHistory, h.Usage, r, a, stdReq)

	if !stdReq.Stream {
		result, outErr := completionruntime.ExecuteNonStreamWithRetry(r.Context(), h.DS, a, stdReq, completionruntime.Options{
			RetryEnabled:          true,
			CurrentInputFile:      h.Store,
			ExpertPromptSegment:   h.Store,
			ToolCallRepairEnabled: true,
			ChatHistory:           h.ChatHistory,
		})
		sessionID = result.SessionID
		if outErr != nil {
			if historySession != nil {
				if completionruntime.IsDirectTokenAuthError(outErr) {
					historySession.discard()
				} else {
					historySession.error(outErr.Status, outErr.Message, outErr.Code, historyThinkingForArchive(result.Turn.RawThinking, result.Turn.DetectionThinking, result.Turn.Thinking), historyTextForArchive(result.Turn.RawText, result.Turn.Text), assistantturn.IncompleteUsage(result.Turn))
				}
			}
			writeOpenAIErrorWithCode(w, outErr.Status, outErr.Message, outErr.Code)
			return
		}
		respBody := openaifmt.BuildChatCompletionFromTurn(result.SessionID, stdReq.ResponseModel, result.Turn)
		respBody["usage"] = assistantturn.OpenAIChatUsage(result.Turn)
		finishReason := assistantturn.FinalizeTurn(result.Turn, assistantturn.FinalizeOptions{}).FinishReason
		if historySession != nil {
			historySession.success(http.StatusOK, historyThinkingForArchive(result.Turn.RawThinking, result.Turn.DetectionThinking, result.Turn.Thinking), historyTextForArchive(result.Turn.RawText, result.Turn.Text), finishReason, assistantturn.OpenAIChatUsage(result.Turn))
		}
		writeJSON(w, http.StatusOK, respBody)
		return
	}

	start, outErr := completionruntime.StartCompletion(r.Context(), h.DS, a, stdReq, completionruntime.Options{
		CurrentInputFile:    h.Store,
		ExpertPromptSegment: h.Store,
	})
	sessionID = start.SessionID
	if outErr != nil {
		if historySession != nil {
			if completionruntime.IsDirectTokenAuthError(outErr) {
				historySession.discard()
			} else {
				historySession.error(outErr.Status, outErr.Message, outErr.Code, "", "")
			}
		}
		writeOpenAIErrorWithCode(w, outErr.Status, outErr.Message, outErr.Code)
		return
	}
	streamReq := start.Request
	refFileTokens := streamReq.RefFileTokens
	h.handleStreamWithRetry(w, r, a, start.Response, start.Payload, start.Pow, sessionID, &sessionID, streamReq, streamReq.ResponseModel, streamReq.PromptTokenText, refFileTokens, streamReq.Thinking, streamReq.Search, streamReq.ToolNames, streamReq.ToolsRaw, streamReq.ToolChoice, historySession)
}

func (h *Handler) handleNonStream(w http.ResponseWriter, resp *http.Response, completionID, model, finalPrompt string, refFileTokens int, thinkingEnabled, searchEnabled bool, toolNames []string, toolsRaw any, historySession *chatHistorySession) {
	if resp.StatusCode != http.StatusOK {
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		if historySession != nil {
			historySession.error(resp.StatusCode, string(body), "error", "", "")
		}
		writeOpenAIError(w, resp.StatusCode, string(body))
		return
	}
	result := sse.CollectStream(resp, thinkingEnabled, true)

	turn := assistantturn.BuildTurnFromCollected(result, assistantturn.BuildOptions{
		Model:         model,
		Prompt:        finalPrompt,
		RefFileTokens: refFileTokens,
		SearchEnabled: searchEnabled,
		ToolNames:     toolNames,
		ToolsRaw:      toolsRaw,
		ToolChoice:    promptcompat.DefaultToolChoicePolicy(),
	})
	outcome := assistantturn.FinalizeTurn(turn, assistantturn.FinalizeOptions{})
	if outcome.ShouldFail {
		status, message, code := outcome.Error.Status, outcome.Error.Message, outcome.Error.Code
		if historySession != nil {
			historySession.error(status, message, code, historyThinkingForArchive(turn.RawThinking, turn.DetectionThinking, turn.Thinking), historyTextForArchive(turn.RawText, turn.Text), assistantturn.IncompleteUsage(turn))
		}
		writeOpenAIErrorWithCode(w, status, message, code)
		return
	}
	respBody := openaifmt.BuildChatCompletionWithToolCalls(completionID, model, finalPrompt, turn.Thinking, turn.Text, turn.ToolCalls, toolsRaw)
	respBody["usage"] = assistantturn.OpenAIChatUsage(turn)
	if historySession != nil {
		historySession.success(http.StatusOK, historyThinkingForArchive(turn.RawThinking, turn.DetectionThinking, turn.Thinking), historyTextForArchive(turn.RawText, turn.Text), outcome.FinishReason, assistantturn.OpenAIChatUsage(turn))
	}
	writeJSON(w, http.StatusOK, respBody)
}

func (h *Handler) handleStream(w http.ResponseWriter, r *http.Request, resp *http.Response, completionID, model, finalPrompt string, refFileTokens int, thinkingEnabled, searchEnabled bool, toolNames []string, toolsRaw any, historySession *chatHistorySession) {
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if historySession != nil {
			historySession.error(resp.StatusCode, string(body), "error", "", "")
		}
		writeOpenAIError(w, resp.StatusCode, string(body))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	rc := http.NewResponseController(w)
	_, canFlush := w.(http.Flusher)
	if !canFlush {
		config.Logger.Warn("[stream] response writer does not support flush; streaming may be buffered")
	}

	created := time.Now().Unix()
	// 无论客户端是否传 tools，都启用 toolSieve 流式拦截：
	// 模型在提示词中被要求使用 <|EPSE|tool_calls> 格式输出工具调用，
	// 若客户端未传 tools（如 opencode 的“继续会话”请求），bufferToolContent
	// 为 false 会导致 EPSE 原文作为正文透传给客户端，产生乱码。
	// toolSieve 解析不依赖 toolNames 过滤，空列表也能正常拦截。
	bufferToolContent := true
	emitEarlyToolDeltas := h.toolcallFeatureMatchEnabled() && h.toolcallEarlyEmitHighConfidence()
	stripReferenceMarkers := stripReferenceMarkersEnabled()
	initialType := "text"
	if thinkingEnabled {
		initialType = "thinking"
	}

	streamRuntime := newChatStreamRuntime(
		w,
		rc,
		canFlush,
		completionID,
		created,
		model,
		finalPrompt,
		thinkingEnabled,
		searchEnabled,
		stripReferenceMarkers,
		toolNames,
		toolsRaw,
		promptcompat.DefaultToolChoicePolicy(),
		bufferToolContent,
		emitEarlyToolDeltas,
	)
	streamRuntime.refFileTokens = refFileTokens

	streamengine.ConsumeSSE(streamengine.ConsumeConfig{
		Context:             r.Context(),
		Body:                resp.Body,
		ThinkingEnabled:     thinkingEnabled,
		InitialType:         initialType,
		KeepAliveInterval:   time.Duration(dsprotocol.KeepAliveTimeout) * time.Second,
		IdleTimeout:         time.Duration(dsprotocol.StreamIdleTimeout) * time.Second,
		MaxKeepAliveNoInput: dsprotocol.MaxKeepaliveCount,
	}, streamengine.ConsumeHooks{
		OutputError: streamRuntime.output.Err,
		OnKeepAlive: streamRuntime.sendKeepAlive,
		OnParsed: func(parsed sse.LineResult) streamengine.ParsedDecision {
			decision := streamRuntime.onParsed(parsed)
			if historySession != nil {
				historySession.progress(streamRuntime.historyThinking(), streamRuntime.historyText())
			}
			return decision
		},
		OnFinalize: func(reason streamengine.StopReason, _ error) {
			if string(reason) == "content_filter" {
				streamRuntime.finalize("content_filter", false)
			} else {
				streamRuntime.finalize("stop", false)
			}
			if historySession == nil {
				return
			}
			if streamRuntime.finalErrorMessage != "" {
				historySession.errorWithUpstream(streamRuntime.finalErrorStatus, streamRuntime.finalErrorMessage, streamRuntime.finalErrorCode, streamRuntime.historyThinking(), streamRuntime.historyText(), streamRuntime.accumulator.UpstreamUsage, promptcompat.StandardRequest{PromptTokenText: streamRuntime.finalPrompt, RefFileTokens: streamRuntime.refFileTokens})
				return
			}
			historySession.success(http.StatusOK, streamRuntime.historyThinking(), streamRuntime.historyText(), streamRuntime.finalFinishReason, streamRuntime.finalUsage)
		},
		OnContextDone: func() {
			streamRuntime.markContextCancelled()
			if historySession != nil {
				historySession.stoppedWithUpstream(streamRuntime.historyThinking(), streamRuntime.historyText(), string(streamengine.StopReasonContextCancelled), streamRuntime.accumulator.UpstreamUsage, promptcompat.StandardRequest{PromptTokenText: streamRuntime.finalPrompt, RefFileTokens: streamRuntime.refFileTokens})
			}
		},
	})
}
