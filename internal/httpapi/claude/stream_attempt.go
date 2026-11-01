package claude

import (
	"net/http"

	"ds2api/internal/completionruntime"
	"ds2api/internal/config"
	"ds2api/internal/promptcompat"
	"ds2api/internal/responsehistory"
	streamengine "ds2api/internal/stream"
)

func (h *Handler) consumeClaudeStreamAttempt(r *http.Request, resp *http.Response, s *claudeStreamRuntime, thinkingEnabled, allowDeferEmpty bool) completionruntime.ConsumeAttemptResult {
	defer func() {
		if err := resp.Body.Close(); err != nil {
			config.Logger.Warn("[claude_stream] close response failed", "error", err)
		}
	}()
	s.resetContentPolicy()
	s.upstreamErr = ""
	initialType := "text"
	if thinkingEnabled && s.rawText.Len() == 0 {
		initialType = "thinking"
	}
	var finalReason streamengine.StopReason
	var scannerErr error
	streamengine.ConsumeSSE(streamengine.ConsumeConfig{
		Context: r.Context(), Body: resp.Body, ThinkingEnabled: thinkingEnabled, InitialType: initialType,
		KeepAliveInterval: claudeStreamPingInterval, IdleTimeout: claudeStreamIdleTimeout, MaxKeepAliveNoInput: claudeStreamMaxKeepaliveCnt,
	}, streamengine.ConsumeHooks{
		OnKeepAlive: s.sendPing,
		OnParsed:    s.onParsed,
		OnFinalize:  func(reason streamengine.StopReason, err error) { finalReason, scannerErr = reason, err },
	})
	if r.Context().Err() != nil {
		if s.history != nil {
			s.history.Stopped(responsehistory.ThinkingForArchive(s.rawThinking.String(), s.toolDetectionThinking.String(), s.thinking.String()), responsehistory.TextForArchive(s.rawText.String(), s.text.String()), string(streamengine.StopReasonContextCancelled), promptcompat.StandardRequest{PromptTokenText: s.promptTokenText})
		}
		return completionruntime.ConsumeAttemptResult{Terminal: true}
	}
	partial := s.rawText.Len() > 0 || s.rawThinking.Len() > 0 || s.toolDetectionThinking.Len() > 0
	if completionruntime.StreamInterrupted(scannerErr, finalReason) && partial {
		if s.responseMessageID > 0 {
			return completionruntime.ConsumeAttemptResult{Retryable: true, ResumeContinue: true}
		}
		s.failRetry(http.StatusBadGateway, "Upstream stream interrupted before completion and cannot be resumed.", "upstream_interrupted")
		return completionruntime.ConsumeAttemptResult{Terminal: true}
	}
	if s.finalize("end_turn", allowDeferEmpty && !s.contentFilter) {
		return completionruntime.ConsumeAttemptResult{Terminal: true}
	}
	return completionruntime.ConsumeAttemptResult{Retryable: true}
}
