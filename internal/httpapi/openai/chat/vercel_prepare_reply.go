package chat

import (
	"encoding/json"
	"io"
	"net/http"

	"ds2api/internal/config"
)

// Failed or canceled prepare responses must not leave the managed account
// leased. The caller's defer releases auth; TTL remains the network-race net.
func (h *Handler) writePreparedLease(w http.ResponseWriter, r *http.Request, leaseID string, payload map[string]any) bool {
	if a := h.lookupStreamLeaseAuth(leaseID); a != nil {
		payload["account_id"] = a.AccountID
	}
	data, err := json.Marshal(payload)
	if err == nil {
		err = r.Context().Err()
	}
	if err == nil {
		w.Header().Set("Content-Type", "application/json")
		var n int
		n, err = w.Write(data)
		if err == nil && n != len(data) {
			err = io.ErrShortWrite
		}
		if err == nil {
			err = r.Context().Err()
		}
	}
	if err == nil {
		return true
	}
	h.releaseStreamLease(leaseID)
	config.Logger.Warn("[vercel_prepare] delivery failed; lease removed", "error", err)
	return false
}
