package chat

import (
        "context"
        "encoding/json"
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
        "ds2api/internal/sessionstate"
        "ds2api/internal/sse"
        streamengine "ds2api/internal/stream"
)

func (h *Handler) ChatCompletions(w http.ResponseWriter, r *http.Request) {
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

        a, err := h.Auth.Determine(r)
        if err != nil {
                status := http.StatusUnauthorized
                detail := err.Error()
                if err == auth.ErrNoAccount {
                        status = http.StatusTooManyRequests
                }
                writeOpenAIError(w, status, detail)
                return
        }
        var sessionID string
        var resolvedModel string
        defer func() {
                h.autoDeleteRemoteSession(r.Context(), a, sessionID, resolvedModel)
                h.Auth.Release(a)
        }()

        r = r.WithContext(auth.WithAuth(r.Context(), a))

        r.Body = http.MaxBytesReader(w, r.Body, openAIGeneralMaxSize)
        var req map[string]any
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
                if strings.Contains(strings.ToLower(err.Error()), "too large") {
                        writeOpenAIError(w, http.StatusRequestEntityTooLarge, "request body too large")
                        return
                }
                writeOpenAIError(w, http.StatusBadRequest, "invalid json")
                return
        }
        if err := h.preprocessInlineFileInputs(r.Context(), a, req); err != nil {
                writeOpenAIInlineFileError(w, err)
                return
        }
        stdReq, err := promptcompat.NormalizeOpenAIChatRequest(h.Store, req, requestTraceID(r))
        if err != nil {
                writeOpenAIError(w, http.StatusBadRequest, err.Error())
                return
        }
        resolvedModel = stdReq.ResolvedModel
        stdReq, err = h.applyCurrentInputFile(r.Context(), a, stdReq)
        if err != nil {
                status, message := mapCurrentInputFileError(err)
                writeOpenAIError(w, status, message)
                return
        }

        // ===== edit_message 会话复用逻辑 =====
        // 条件：模型带 -v4f 后缀 且 未启用历史拆分（edit_message 无法修改文件引用）
        // 且 有可复用的会话状态（fingerprint 匹配 + 同账号 + 未超 edit 次数上限）
        if stdReq.EditReuseEnabled && !stdReq.CurrentInputFileApplied && stdReq.MessagesFingerprint != "" && stdReq.LastUserMessage != "" {
                if editState := globalSessionState.Get(stdReq.MessagesFingerprint); editState != nil && editState.AccountID == a.AccountID {
                        // 尝试通过 edit_message 复用远端会话
                        h.handleEditReuse(w, r, a, stdReq, editState, &sessionID)
                        return
                }
        }

        // ===== 常规路径：新建会话 → completion =====
        historySession := startChatHistory(h.ChatHistory, r, a, stdReq)

        if !stdReq.Stream {
                result, outErr := completionruntime.ExecuteNonStreamWithRetry(r.Context(), h.DS, a, stdReq, completionruntime.Options{
                        RetryEnabled:     true,
                        CurrentInputFile: h.Store,
                })
                sessionID = result.SessionID
                if outErr != nil {
                        if historySession != nil {
                                historySession.error(outErr.Status, outErr.Message, outErr.Code, historyThinkingForArchive(result.Turn.RawThinking, result.Turn.DetectionThinking, result.Turn.Thinking), historyTextForArchive(result.Turn.RawText, result.Turn.Text))
                        }
                        writeOpenAIErrorWithCode(w, outErr.Status, outErr.Message, outErr.Code)
                        return
                }
                respBody := openaifmt.BuildChatCompletionWithToolCalls(result.SessionID, stdReq.ResponseModel, result.Turn.Prompt, result.Turn.Thinking, result.Turn.Text, result.Turn.ToolCalls, stdReq.ToolsRaw)
                respBody["usage"] = assistantturn.OpenAIChatUsage(result.Turn)
                finishReason := assistantturn.FinalizeTurn(result.Turn, assistantturn.FinalizeOptions{}).FinishReason
                // 保存会话状态用于后续 edit_message 复用
                h.saveSessionStateForEditReuse(stdReq, result.SessionID, result.Turn.ResponseMessageID, a.AccountID)
                if historySession != nil {
                        historySession.success(http.StatusOK, historyThinkingForArchive(result.Turn.RawThinking, result.Turn.DetectionThinking, result.Turn.Thinking), historyTextForArchive(result.Turn.RawText, result.Turn.Text), finishReason, assistantturn.OpenAIChatUsage(result.Turn))
                }
                writeJSON(w, http.StatusOK, respBody)
                return
        }

        start, outErr := completionruntime.StartCompletion(r.Context(), h.DS, a, stdReq, completionruntime.Options{
                CurrentInputFile: h.Store,
        })
        sessionID = start.SessionID
        if outErr != nil {
                if historySession != nil {
                        historySession.error(outErr.Status, outErr.Message, outErr.Code, "", "")
                }
                writeOpenAIErrorWithCode(w, outErr.Status, outErr.Message, outErr.Code)
                return
        }
        streamReq := start.Request
        refFileTokens := streamReq.RefFileTokens
        h.handleStreamWithRetry(w, r, a, start.Response, start.Payload, start.Pow, sessionID, &sessionID, streamReq, streamReq.ResponseModel, streamReq.PromptTokenText, refFileTokens, streamReq.Thinking, streamReq.Search, streamReq.ToolNames, streamReq.ToolsRaw, streamReq.ToolChoice, historySession)
}

// autoDeleteRemoteSession 在请求结束后按全局 auto_delete.mode 与模型后缀策略
// 清理远端会话。
//
// 策略组合：
//   - 全局 mode=single：删除本次会话（无条件，等价于既有行为）
//   - 全局 mode=all：清空账号全部会话（无条件，等价于既有行为）
//   - 全局 mode=none + 模型带 -autodelete 后缀：降级为删除本次会话
//   - 全局 mode=none + 普通模型：不删除
//
// 设计意图：-autodelete 后缀模型让用户可以在全局关闭删除策略的前提下，
// 对个别请求按需启用「仅删除本次对话」，对应 App 抓包里的
// POST /api/v0/chat_session/delete 动作。
func (h *Handler) autoDeleteRemoteSession(ctx context.Context, a *auth.RequestAuth, sessionID string, resolvedModel string) {
        mode := h.Store.AutoDeleteMode()
        suffixAutoDelete := config.IsAutoDeleteModel(resolvedModel)

        // edit_message 复用模型：保留远端会话，不删除，以便后续 edit_message 复用。
        // 远端会话有自然的 TTL 过期机制，sessionstate 也有 30 分钟本地 TTL。
        // 只有当用户同时指定 -autodelete 时，才仍然删除（显式覆盖）。
        if config.IsEditReuseModel(resolvedModel) && !suffixAutoDelete {
                config.Logger.Debug("[auto_delete_sessions] skipped for edit-reuse model", "account", a.AccountID, "model", resolvedModel, "session_id", sessionID)
                return
        }

        // 计算最终生效模式
        effectiveMode := mode
        if mode == "none" && suffixAutoDelete {
                effectiveMode = "single"
        }
        if effectiveMode == "none" || a.DeepSeekToken == "" {
                return
        }

        deleteBaseCtx := context.WithoutCancel(ctx)
        deleteCtx, cancel := context.WithTimeout(deleteBaseCtx, 10*time.Second)
        defer cancel()

        switch effectiveMode {
        case "single":
                if sessionID == "" {
                        config.Logger.Warn("[auto_delete_sessions] skipped single-session delete because session_id is empty", "account", a.AccountID, "model", resolvedModel)
                        return
                }
                _, err := h.DS.DeleteSessionForToken(deleteCtx, a.DeepSeekToken, sessionID)
                if err != nil {
                        config.Logger.Warn("[auto_delete_sessions] failed", "account", a.AccountID, "mode", mode, "effective_mode", effectiveMode, "model", resolvedModel, "session_id", sessionID, "error", err)
                        return
                }
                config.Logger.Debug("[auto_delete_sessions] success", "account", a.AccountID, "mode", mode, "effective_mode", effectiveMode, "model", resolvedModel, "session_id", sessionID)
        case "all":
                if err := h.DS.DeleteAllSessionsForToken(deleteCtx, a.DeepSeekToken); err != nil {
                        config.Logger.Warn("[auto_delete_sessions] failed", "account", a.AccountID, "mode", mode, "model", resolvedModel, "error", err)
                        return
                }
                config.Logger.Debug("[auto_delete_sessions] success", "account", a.AccountID, "mode", mode, "model", resolvedModel)
        default:
                config.Logger.Warn("[auto_delete_sessions] unknown mode", "account", a.AccountID, "mode", mode, "model", resolvedModel)
        }
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
                        historySession.error(status, message, code, historyThinkingForArchive(turn.RawThinking, turn.DetectionThinking, turn.Thinking), historyTextForArchive(turn.RawText, turn.Text))
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
        bufferToolContent := len(toolNames) > 0
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
                OnKeepAlive: func() {
                        streamRuntime.sendKeepAlive()
                },
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
                                historySession.error(streamRuntime.finalErrorStatus, streamRuntime.finalErrorMessage, streamRuntime.finalErrorCode, streamRuntime.historyThinking(), streamRuntime.historyText())
                                return
                        }
                        historySession.success(http.StatusOK, streamRuntime.historyThinking(), streamRuntime.historyText(), streamRuntime.finalFinishReason, streamRuntime.finalUsage)
                },
                OnContextDone: func() {
                        streamRuntime.markContextCancelled()
                        if historySession != nil {
                                historySession.stopped(streamRuntime.historyThinking(), streamRuntime.historyText(), string(streamengine.StopReasonContextCancelled))
                        }
                },
        })
}

// globalSessionState 是全局会话状态缓存，用于 edit_message 复用。
var globalSessionState = sessionstate.NewStore()

// handleEditReuse 处理 edit_message 会话复用路径。
// 当 fingerprint 匹配到活跃会话时，通过 edit_message 仅发送最新用户消息，
// 而非新建会话重发完整对话。如果 edit_message 返回 edit_limit 错误，
// 自动回退到常规新建会话路径。
func (h *Handler) handleEditReuse(w http.ResponseWriter, r *http.Request, a *auth.RequestAuth, stdReq promptcompat.StandardRequest, editState *sessionstate.State, sessionIDPtr *string) {
        fingerprint := stdReq.MessagesFingerprint

        // 获取 PoW
        pow, err := h.DS.GetPow(r.Context(), a, 3)
        if err != nil {
                config.Logger.Warn("[edit_reuse] PoW failed, falling back to new session", "error", err, "account", a.AccountID)
                globalSessionState.Invalidate(fingerprint)
                h.handleFallbackNewSession(w, r, a, stdReq, sessionIDPtr)
                return
        }

        // 构建 edit_message payload
        payload := stdReq.EditMessagePayload(editState.SessionID, editState.RequestMessageID)
        *sessionIDPtr = editState.SessionID

        config.Logger.Info("[edit_reuse] attempting edit_message",
                "session_id", editState.SessionID,
                "message_id", editState.RequestMessageID,
                "edit_count", editState.EditCount,
                "account", a.AccountID,
                "prompt_len", len(stdReq.LastUserMessage),
        )

        resp, err := h.DS.CallEditMessage(r.Context(), a, payload, pow, 3)
        if err != nil {
                config.Logger.Warn("[edit_reuse] CallEditMessage failed, falling back", "error", err, "account", a.AccountID)
                globalSessionState.Invalidate(fingerprint)
                h.handleFallbackNewSession(w, r, a, stdReq, sessionIDPtr)
                return
        }

        // 检查是否收到 edit_limit 错误
        if resp.StatusCode != http.StatusOK {
                body, _ := io.ReadAll(resp.Body)
                resp.Body.Close()
                bodyStr := string(body)
                config.Logger.Warn("[edit_reuse] edit_message non-200, falling back", "status", resp.StatusCode, "body", bodyStr, "account", a.AccountID)
                globalSessionState.Invalidate(fingerprint)
                h.handleFallbackNewSession(w, r, a, stdReq, sessionIDPtr)
                return
        }

        // 检查 SSE 流中是否包含 edit_limit hint
        // 我们用一种简单的方式：先读取一小段看是否有 edit_limit 错误
        // 如果有，回退到新建会话
        // 对于流式请求，我们需要在 SSE 消费过程中检测

        historySession := startChatHistory(h.ChatHistory, r, a, stdReq)

        if !stdReq.Stream {
                // 非流式：收集完整响应后检查
                h.handleEditReuseNonStream(w, r, a, stdReq, resp, editState, fingerprint, historySession, sessionIDPtr)
        } else {
                // 流式：在 SSE 消费过程中检测 edit_limit
                h.handleEditReuseStream(w, r, a, stdReq, resp, editState, fingerprint, historySession, sessionIDPtr)
        }
}

// handleFallbackNewSession 在 edit_message 失败时回退到新建会话路径。
func (h *Handler) handleFallbackNewSession(w http.ResponseWriter, r *http.Request, a *auth.RequestAuth, stdReq promptcompat.StandardRequest, sessionIDPtr *string) {
        historySession := startChatHistory(h.ChatHistory, r, a, stdReq)

        if !stdReq.Stream {
                result, outErr := completionruntime.ExecuteNonStreamWithRetry(r.Context(), h.DS, a, stdReq, completionruntime.Options{
                        RetryEnabled:     true,
                        CurrentInputFile: h.Store,
                })
                *sessionIDPtr = result.SessionID
                if outErr != nil {
                        if historySession != nil {
                                historySession.error(outErr.Status, outErr.Message, outErr.Code, historyThinkingForArchive(result.Turn.RawThinking, result.Turn.DetectionThinking, result.Turn.Thinking), historyTextForArchive(result.Turn.RawText, result.Turn.Text))
                        }
                        writeOpenAIErrorWithCode(w, outErr.Status, outErr.Message, outErr.Code)
                        return
                }
                // 保存新的会话状态
                if stdReq.EditReuseEnabled && stdReq.MessagesFingerprint != "" && result.Turn.ResponseMessageID > 0 {
                        globalSessionState.Put(stdReq.MessagesFingerprint, &sessionstate.State{
                                SessionID:         result.SessionID,
                                RequestMessageID:  result.Turn.ResponseMessageID - 1, // 用户消息 ID = 回复消息 ID - 1（近似）
                                AccountID:         a.AccountID,
                                EditCount:         0,
                        })
                }
                respBody := openaifmt.BuildChatCompletionWithToolCalls(result.SessionID, stdReq.ResponseModel, result.Turn.Prompt, result.Turn.Thinking, result.Turn.Text, result.Turn.ToolCalls, stdReq.ToolsRaw)
                respBody["usage"] = assistantturn.OpenAIChatUsage(result.Turn)
                finishReason := assistantturn.FinalizeTurn(result.Turn, assistantturn.FinalizeOptions{}).FinishReason
                if historySession != nil {
                        historySession.success(http.StatusOK, historyThinkingForArchive(result.Turn.RawThinking, result.Turn.DetectionThinking, result.Turn.Thinking), historyTextForArchive(result.Turn.RawText, result.Turn.Text), finishReason, assistantturn.OpenAIChatUsage(result.Turn))
                }
                writeJSON(w, http.StatusOK, respBody)
                return
        }

        start, outErr := completionruntime.StartCompletion(r.Context(), h.DS, a, stdReq, completionruntime.Options{
                CurrentInputFile: h.Store,
        })
        *sessionIDPtr = start.SessionID
        if outErr != nil {
                if historySession != nil {
                        historySession.error(outErr.Status, outErr.Message, outErr.Code, "", "")
                }
                writeOpenAIErrorWithCode(w, outErr.Status, outErr.Message, outErr.Code)
                return
        }
        streamReq := start.Request
        refFileTokens := streamReq.RefFileTokens
        h.handleStreamWithRetry(w, r, a, start.Response, start.Payload, start.Pow, start.SessionID, sessionIDPtr, streamReq, streamReq.ResponseModel, streamReq.PromptTokenText, refFileTokens, streamReq.Thinking, streamReq.Search, streamReq.ToolNames, streamReq.ToolsRaw, streamReq.ToolChoice, historySession)
}

// handleEditReuseNonStream 处理非流式的 edit_message 响应。
func (h *Handler) handleEditReuseNonStream(w http.ResponseWriter, r *http.Request, a *auth.RequestAuth, stdReq promptcompat.StandardRequest, resp *http.Response, editState *sessionstate.State, fingerprint string, historySession *chatHistorySession, sessionIDPtr *string) {
        defer resp.Body.Close()

        if resp.StatusCode != http.StatusOK {
                body, _ := io.ReadAll(resp.Body)
                h.handleFallbackNewSession(w, r, a, stdReq, sessionIDPtr)
                return
        }

        result := sse.CollectStream(resp, stdReq.Thinking, false)

        // 检查 edit_limit
        if result.EditLimitHit {
                config.Logger.Info("[edit_reuse] edit_limit hit, falling back to new session", "account", a.AccountID)
                globalSessionState.Invalidate(fingerprint)
                h.handleFallbackNewSession(w, r, a, stdReq, sessionIDPtr)
                return
        }

        turn := assistantturn.BuildTurnFromCollected(result, assistantturn.BuildOptions{
                Model:         stdReq.ResponseModel,
                Prompt:        stdReq.PromptTokenText,
                RefFileTokens: stdReq.RefFileTokens,
                SearchEnabled: stdReq.Search,
                ToolNames:     stdReq.ToolNames,
                ToolsRaw:      stdReq.ToolsRaw,
                ToolChoice:    stdReq.ToolChoice,
        })

        outcome := assistantturn.FinalizeTurn(turn, assistantturn.FinalizeOptions{})
        if outcome.ShouldFail {
                status, message, code := outcome.Error.Status, outcome.Error.Message, outcome.Error.Code
                if historySession != nil {
                        historySession.error(status, message, code, historyThinkingForArchive(turn.RawThinking, turn.DetectionThinking, turn.Thinking), historyTextForArchive(turn.RawText, turn.Text))
                }
                writeOpenAIErrorWithCode(w, status, message, code)
                return
        }

        // 更新会话状态：edit_count +1，更新 request_message_id
        if result.RequestMessageID > 0 {
                globalSessionState.IncrementEditCount(fingerprint, result.RequestMessageID)
        }

        respBody := openaifmt.BuildChatCompletionWithToolCalls(editState.SessionID, stdReq.ResponseModel, turn.Prompt, turn.Thinking, turn.Text, turn.ToolCalls, stdReq.ToolsRaw)
        respBody["usage"] = assistantturn.OpenAIChatUsage(turn)
        if historySession != nil {
                historySession.success(http.StatusOK, historyThinkingForArchive(turn.RawThinking, turn.DetectionThinking, turn.Thinking), historyTextForArchive(turn.RawText, turn.Text), outcome.FinishReason, assistantturn.OpenAIChatUsage(turn))
        }
        writeJSON(w, http.StatusOK, respBody)
}

// handleEditReuseStream 处理流式的 edit_message 响应。
func (h *Handler) handleEditReuseStream(w http.ResponseWriter, r *http.Request, a *auth.RequestAuth, stdReq promptcompat.StandardRequest, resp *http.Response, editState *sessionstate.State, fingerprint string, historySession *chatHistorySession, sessionIDPtr *string) {
        if resp.StatusCode != http.StatusOK {
                resp.Body.Close()
                h.handleFallbackNewSession(w, r, a, stdReq, sessionIDPtr)
                return
        }

        w.Header().Set("Content-Type", "text/event-stream")
        w.Header().Set("Cache-Control", "no-cache, no-transform")
        w.Header().Set("Connection", "keep-alive")
        w.Header().Set("X-Accel-Buffering", "no")
        rc := http.NewResponseController(w)
        _, canFlush := w.(http.Flusher)

        created := time.Now().Unix()
        thinkingEnabled := stdReq.Thinking
        searchEnabled := stdReq.Search
        stripReferenceMarkers := stripReferenceMarkersEnabled()
        bufferToolContent := len(stdReq.ToolNames) > 0
        emitEarlyToolDeltas := h.toolcallFeatureMatchEnabled() && h.toolcallEarlyEmitHighConfidence()
        initialType := "text"
        if thinkingEnabled {
                initialType = "thinking"
        }

        streamRuntime := newChatStreamRuntime(
                w, rc, canFlush,
                editState.SessionID, // completionID 使用 sessionID
                created,
                stdReq.ResponseModel,
                stdReq.PromptTokenText,
                thinkingEnabled, searchEnabled, stripReferenceMarkers,
                stdReq.ToolNames, stdReq.ToolsRaw, stdReq.ToolChoice,
                bufferToolContent, emitEarlyToolDeltas,
        )
        streamRuntime.refFileTokens = stdReq.RefFileTokens

        // 标记是否检测到 edit_limit
        editLimitDetected := false

        streamengine.ConsumeSSE(streamengine.ConsumeConfig{
                Context:             r.Context(),
                Body:                resp.Body,
                ThinkingEnabled:     thinkingEnabled,
                InitialType:         initialType,
                KeepAliveInterval:   time.Duration(dsprotocol.KeepAliveTimeout) * time.Second,
                IdleTimeout:         time.Duration(dsprotocol.StreamIdleTimeout) * time.Second,
                MaxKeepAliveNoInput: dsprotocol.MaxKeepaliveCount,
        }, streamengine.ConsumeHooks{
                OnKeepAlive: func() {
                        streamRuntime.sendKeepAlive()
                },
                OnParsed: func(parsed sse.LineResult) streamengine.ParsedDecision {
                        // 检测 edit_limit hint 事件
                        if parsed.EditLimitHit {
                                editLimitDetected = true
                                config.Logger.Info("[edit_reuse] edit_limit detected in stream, stopping", "account", a.AccountID)
                                return streamengine.ParsedDecision{Stop: true, StopReason: streamengine.StopReason("edit_limit")}
                        }
                        decision := streamRuntime.onParsed(parsed)
                        if historySession != nil {
                                historySession.progress(streamRuntime.historyThinking(), streamRuntime.historyText())
                        }
                        return decision
                },
                OnFinalize: func(reason streamengine.StopReason, _ error) {
                        if editLimitDetected {
                                // edit_limit 被检测到，使失效缓存
                                globalSessionState.Invalidate(fingerprint)
                                config.Logger.Info("[edit_reuse] edit_limit finalized, session state invalidated", "account", a.AccountID)
                                // 向客户端返回一个错误提示
                                streamRuntime.sendFailedChunk(http.StatusTooManyRequests, "edit_limit: message edit count exceeded, please retry (new session will be created)", "edit_limit")
                                return
                        }
                        if string(reason) == "content_filter" {
                                streamRuntime.finalize("content_filter", false)
                        } else {
                                streamRuntime.finalize("stop", false)
                        }
                        // 更新会话状态
                        if streamRuntime.responseMessageID > 0 && streamRuntime.responseMessageID > 1 {
                                newReqMsgID := streamRuntime.responseMessageID - 1 // 近似
                                if streamRuntime.firstChunkSent {
                                        // 响应成功，更新 edit count
                                        globalSessionState.IncrementEditCount(fingerprint, newReqMsgID)
                                }
                        }
                        if historySession == nil {
                                return
                        }
                        if streamRuntime.finalErrorMessage != "" {
                                historySession.error(streamRuntime.finalErrorStatus, streamRuntime.finalErrorMessage, streamRuntime.finalErrorCode, streamRuntime.historyThinking(), streamRuntime.historyText())
                                return
                        }
                        historySession.success(http.StatusOK, streamRuntime.historyThinking(), streamRuntime.historyText(), streamRuntime.finalFinishReason, streamRuntime.finalUsage)
                },
                OnContextDone: func() {
                        streamRuntime.markContextCancelled()
                        if historySession != nil {
                                historySession.stopped(streamRuntime.historyThinking(), streamRuntime.historyText(), string(streamengine.StopReasonContextCancelled))
                        }
                },
        })
}

// saveSessionStateForEditReuse 在首次 completion 成功后保存会话状态，
// 使得后续同一对话上下文的请求可以通过 edit_message 复用此会话。
func (h *Handler) saveSessionStateForEditReuse(stdReq promptcompat.StandardRequest, sessionID string, responseMessageID int, accountID string) {
        if !stdReq.EditReuseEnabled || stdReq.MessagesFingerprint == "" || sessionID == "" || responseMessageID <= 0 {
                return
        }
        // 不保存使用了历史拆分的请求（edit_message 无法修改文件引用）
        if stdReq.CurrentInputFileApplied {
                return
        }
        // responseMessageID 是模型回复的 message_id，用户消息的 message_id = responseMessageID - 1
        requestMessageID := responseMessageID - 1
        if requestMessageID <= 0 {
                requestMessageID = 1
        }
        globalSessionState.Put(stdReq.MessagesFingerprint, &sessionstate.State{
                SessionID:         sessionID,
                RequestMessageID:  requestMessageID,
                AccountID:         accountID,
                EditCount:         0,
        })
        config.Logger.Info("[edit_reuse] saved session state for future reuse",
                "session_id", sessionID,
                "request_message_id", requestMessageID,
                "account", accountID,
                "fingerprint", stdReq.MessagesFingerprint[:12]+"...",
        )
}
