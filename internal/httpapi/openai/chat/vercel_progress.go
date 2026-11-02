package chat

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	"ds2api/internal/assistantturn"
	"ds2api/internal/config"
	"ds2api/internal/tooldiagnostics"
)

func (h *Handler) leaseDiagnosticProgressEnabled(id string) bool {
	lease, ok := h.lookupStreamLease(id)
	return ok && lease.Diagnostics != nil
}

// Progress carries evidence only; the authenticated, lease-bound bridge cannot
// use it to validate/execute calls or trigger model repair.
func (h *Handler) handleVercelStreamProgress(w http.ResponseWriter, r *http.Request) {
	if !config.IsVercel() {
		http.NotFound(w, r)
		return
	}
	secret := vercelInternalSecret()
	provided := strings.TrimSpace(r.Header.Get("X-Ds2-Internal-Token"))
	if secret == "" || subtle.ConstantTimeCompare([]byte(secret), []byte(provided)) != 1 {
		writeOpenAIError(w, 401, "unauthorized internal request")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16*tooldiagnostics.MaxTextBytes)
	var body struct {
		LeaseID  string               `json:"lease_id"`
		Attempt  int                  `json:"diagnostic_attempt"`
		Reason   string               `json:"diagnostic_retry_reason"`
		Content  tooldiagnostics.Text `json:"content"`
		Thinking tooldiagnostics.Text `json:"thinking"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Attempt < 1 || body.Attempt > 32 {
		writeOpenAIError(w, 400, "invalid progress")
		return
	}
	lease, ok := h.lookupStreamLease(body.LeaseID)
	if !ok {
		writeOpenAIError(w, 404, "stream lease not found")
		return
	}
	ctx := leaseDiagnosticContext(r.Context(), lease, map[string]any{"diagnostic_attempt": body.Attempt, "diagnostic_retry_reason": body.Reason})
	assistantturn.CheckpointToolProgress(ctx, lease.Standard.ToolMarker, body.Content, body.Thinking)
	writeJSON(w, 200, map[string]bool{"success": true})
}
