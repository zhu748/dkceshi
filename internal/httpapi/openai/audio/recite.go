package audio

import (
	"context"
	"net/http"
	"strings"

	"ds2api/internal/assistantturn"
	"ds2api/internal/auth"
	"ds2api/internal/completionruntime"
	"ds2api/internal/promptcompat"
)

type reciteResult struct {
	SessionID string
	MessageID int
	Text      string
}

// recite asks the web model to copy the input verbatim, then returns the
// assistant message id that the TTS WebSocket can read.
func (h *Handler) recite(ctx context.Context, a *auth.RequestAuth, model, input string) (reciteResult, *assistantturn.OutputError) {
	prompt := buildRecitePrompt(input)
	stdReq := promptcompat.StandardRequest{
		Surface:         ttsReciteSurface,
		RequestedModel:  model,
		ResolvedModel:   model,
		ResponseModel:   model,
		Messages:        []any{map[string]any{"role": "user", "content": prompt}},
		FinalPrompt:     prompt,
		PromptTokenText: prompt,
		Thinking:        false,
		Search:          false,
	}
	result, outErr := completionruntime.ExecuteNonStreamWithRetry(ctx, h.DS, a, stdReq, completionruntime.Options{
		MaxAttempts:  3,
		RetryEnabled: true,
	})
	if outErr != nil {
		return reciteResult{SessionID: result.SessionID}, outErr
	}
	if strings.TrimSpace(result.Turn.Text) == "" {
		return reciteResult{SessionID: result.SessionID}, &assistantturn.OutputError{
			Status:  http.StatusBadGateway,
			Message: "upstream recitation produced empty text",
			Code:    "tts_empty_recitation",
		}
	}
	// TTS reads the assistant message the completion started with. When the
	// reply was long enough to trigger auto-continue, ResponseMessageID names
	// the last continuation segment instead, and the TTS endpoint answers that
	// one with service_error.
	messageID := result.Turn.FirstResponseMessageID
	if messageID <= 0 {
		messageID = result.Turn.ResponseMessageID
	}
	if messageID <= 0 {
		return reciteResult{SessionID: result.SessionID}, &assistantturn.OutputError{
			Status:  http.StatusBadGateway,
			Message: "upstream did not return a response message id",
			Code:    "tts_upstream_error",
		}
	}
	return reciteResult{
		SessionID: result.SessionID,
		MessageID: messageID,
		Text:      result.Turn.Text,
	}, nil
}

func buildRecitePrompt(input string) string {
	return "请把下面的内容原样复述一遍，用于语音朗读。要求：\n" +
		"1. 只输出复述内容，不要解释、不要加引号、不要用代码块。\n" +
		"2. 保留原文的文字和顺序，不要翻译、不要总结、不要改写。\n" +
		"3. 如果原文包含 Markdown 标记（例如 **、#、`），去掉标记，只保留可朗读的文字。\n\n" +
		input
}
