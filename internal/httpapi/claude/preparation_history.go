package claude

import (
	"net/http"
	"time"

	"ds2api/internal/auth"
	"ds2api/internal/httpapi/openai/files"
	"ds2api/internal/promptcompat"
	"ds2api/internal/responsehistory"
)

func (h *Handler) writePreparationError(w http.ResponseWriter, r *http.Request, a *auth.RequestAuth, standard promptcompat.StandardRequest, cause error, startedAt time.Time, inline bool) {
	status, message := mapCurrentInputFileError(cause)
	if inline {
		status, message = files.InlineFileErrorDetails(cause)
	}
	responsehistory.RecordPreparationError(responsehistory.StartParams{
		Store: h.ChatHistory, Request: r, Auth: a, Surface: "claude.messages", Standard: standard, StartedAt: startedAt,
	}, status, message, cause)
	writeClaudeError(w, status, message)
}
