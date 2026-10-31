package claude

import "net/http"

// writeClaudeError emits an Anthropic-spec error body:
// {"type":"error","error":{"type":"...","message":"..."}}.
//
// The error type follows the Anthropic status mapping (401 -> authentication_error,
// 403 -> permission_error, 404 -> not_found_error, 413 -> request_too_large,
// 429 -> rate_limit_error, 5xx -> api_error; everything else is invalid_request_error)
// and matches the streaming error frames written by stream_runtime_emit.go.
func writeClaudeError(w http.ResponseWriter, status int, message string) {
	errorType := "invalid_request_error"
	switch status {
	case http.StatusUnauthorized:
		errorType = "authentication_error"
	case http.StatusForbidden:
		errorType = "permission_error"
	case http.StatusNotFound:
		errorType = "not_found_error"
	case http.StatusRequestEntityTooLarge:
		errorType = "request_too_large"
	case http.StatusTooManyRequests:
		errorType = "rate_limit_error"
	case http.StatusServiceUnavailable:
		errorType = "overloaded_error"
	default:
		if status >= 500 {
			errorType = "api_error"
		}
	}
	writeJSON(w, status, map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    errorType,
			"message": message,
		},
	})
}
