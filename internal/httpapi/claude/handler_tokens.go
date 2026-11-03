package claude

import (
	"errors"
	"net/http"

	"ds2api/internal/httpapi/openai/files"
	"ds2api/internal/httpapi/openai/shared"
	"ds2api/internal/jsonvalue"
)

func (h *Handler) CountTokens(w http.ResponseWriter, r *http.Request) {
	a, err := h.Auth.Determine(r)
	if err != nil {
		writeClaudeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	defer h.Auth.Release(a)

	var req map[string]any
	r.Body = http.MaxBytesReader(w, r.Body, shared.GeneralMaxSize)
	if err := jsonvalue.Decode(r.Body, &req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeClaudeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		writeClaudeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	model, _ := req["model"].(string)
	messages, _ := req["messages"].([]any)
	if model == "" || len(messages) == 0 {
		writeClaudeError(w, http.StatusBadRequest, "Request must include 'model' and 'messages'.")
		return
	}
	normalized, err := normalizeClaudeRequest(h.Store, req)
	if err != nil {
		writeClaudeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := files.CountFileInputs(r.Context(), a, req, h.DS, normalized.Standard.ResolvedModel, normalized.Standard.Thinking); err != nil {
		status, message := files.InlineFileErrorDetails(err)
		writeClaudeError(w, status, message)
		return
	}
	normalized, err = normalizeClaudeRequest(h.Store, req)
	if err != nil {
		writeClaudeError(w, http.StatusBadRequest, err.Error())
		return
	}
	inputTokens := countClaudeInputTokens(normalized.Standard)
	writeJSON(w, http.StatusOK, map[string]any{"input_tokens": inputTokens})
}
