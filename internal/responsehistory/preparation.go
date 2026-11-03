package responsehistory

import (
	"net/http"
	"strings"

	dsclient "ds2api/internal/deepseek/client"
)

// RecordPreparationError records failures before a completion/history session
// exists. Failed internal prepare requests are included; successful prepare is
// still archived only on release. Failed preparation consumes no model tokens.
func RecordPreparationError(params StartParams, status int, message string, cause error) {
	if cause == nil || dsclient.IsDirectUnauthorizedError(cause) {
		return
	}
	params.Usage = nil
	session := start(params)
	if session == nil {
		return
	}
	if detail := strings.TrimSpace(cause.Error()); detail != "" && !strings.Contains(message, detail) {
		message += ": " + detail
	}
	session.Error(status, message, "file_preparation_error", "", "")
}

func shouldCapture(r *http.Request) bool {
	if r == nil || r.URL == nil {
		return false
	}
	if strings.TrimSpace(r.URL.Query().Get("__stream_prepare")) == "1" {
		return false
	}
	if strings.TrimSpace(r.URL.Query().Get("__stream_release")) == "1" {
		return false
	}
	return true
}
