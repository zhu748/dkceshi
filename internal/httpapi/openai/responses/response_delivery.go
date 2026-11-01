package responses

import (
	"net/http"

	"ds2api/internal/assistantturn"
	"ds2api/internal/responsehistory"
)

// Commit caller-owned continuation state before advertising a stored response.
func writeStoredResponse(w http.ResponseWriter, value map[string]any, persist func(map[string]any) bool, history *responsehistory.Session, turn assistantturn.Turn) {
	if !persist(value) {
		if history != nil {
			history.ErrorTurn(http.StatusInsufficientStorage, responseStorageMessage, responseStorageCode, turn)
		}
		writeOpenAIErrorWithCode(w, http.StatusInsufficientStorage, responseStorageMessage, responseStorageCode)
		return
	}
	if history != nil {
		history.SuccessTurn(http.StatusOK, turn, responsehistory.GenericUsage(turn))
	}
	writeJSON(w, http.StatusOK, value)
}
