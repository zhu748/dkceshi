package chat

import (
	"net/http"
	"time"

	"ds2api/internal/auth"
	"ds2api/internal/httpapi/openai/files"
	"ds2api/internal/promptcompat"
	"ds2api/internal/responsehistory"
	"ds2api/internal/util"
)

func (h *Handler) recordPreparationError(r *http.Request, a *auth.RequestAuth, standard promptcompat.StandardRequest, status int, message string, cause error, startedAt time.Time) {
	responsehistory.RecordPreparationError(responsehistory.StartParams{
		Store: h.ChatHistory, Request: r, Auth: a, Surface: "openai.chat_completions", Standard: standard, StartedAt: startedAt,
	}, status, message, cause)
}

func (h *Handler) writeInlinePreparationError(w http.ResponseWriter, r *http.Request, a *auth.RequestAuth, req map[string]any, cause error, startedAt time.Time) {
	status, message := files.InlineFileErrorDetails(cause)
	standard, err := promptcompat.NormalizeOpenAIChatRequest(h.Store, req, requestTraceID(r), a.ToolMarker)
	if err != nil {
		// Keep the diagnostic even if an incomplete upload cannot be normalized.
		messages, _ := req["messages"].([]any)
		model, _ := req["model"].(string)
		standard = promptcompat.StandardRequest{Messages: messages, ResponseModel: model, ToolsRaw: req["tools"], Stream: util.ToBool(req["stream"])}
	}
	h.recordPreparationError(r, a, standard, status, message, cause, startedAt)
	writeOpenAIInlineFileError(w, cause)
}

func (h *Handler) writeContextPreparationError(w http.ResponseWriter, r *http.Request, a *auth.RequestAuth, standard promptcompat.StandardRequest, cause error, startedAt time.Time) {
	status, message := mapCurrentInputFileError(cause)
	h.recordPreparationError(r, a, standard, status, message, cause, startedAt)
	writeOpenAIError(w, status, message)
}
