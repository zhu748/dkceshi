package tooldiagnostics

import (
	"encoding/json"
	"net/http"

	"ds2api/internal/completionruntime"
	adminshared "ds2api/internal/httpapi/admin/shared"
	"ds2api/internal/tooldiagnostics"
)

func (h *Handler) repairTest(w http.ResponseWriter, r *http.Request) {
	if !h.available(w) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32768)
	var draft tooldiagnostics.RepairSettings
	if err := json.NewDecoder(r.Body).Decode(&draft); err != nil {
		adminshared.WriteJSON(w, 400, map[string]any{"detail": "invalid repair settings"})
		return
	}
	settings, err := h.Store.PreviewRepairGeneration(draft)
	if err != nil {
		h.respondError(w, err)
		return
	}
	result, err := completionruntime.TestRepairAPI(r.Context(), settings)
	w.Header().Set("Cache-Control", "no-store")
	if err != nil {
		adminshared.WriteJSON(w, 502, map[string]any{"detail": err.Error()})
		return
	}
	adminshared.WriteJSON(w, 200, result)
}
