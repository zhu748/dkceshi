package claude

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteClaudeErrorIncludesUnifiedFields(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		wantType   string
		wantTopMsg bool
	}{
		{"unauthorized maps to authentication_error", http.StatusUnauthorized, "authentication_error", true},
		{"forbidden maps to permission_error", http.StatusForbidden, "permission_error", true},
		{"too many requests maps to rate_limit_error", http.StatusTooManyRequests, "rate_limit_error", true},
		{"not found maps to not_found_error", http.StatusNotFound, "not_found_error", true},
		{"bad request maps to invalid_request_error", http.StatusBadRequest, "invalid_request_error", true},
		{"internal error maps to api_error", http.StatusInternalServerError, "api_error", true},
		{"overloaded maps to overloaded_error", http.StatusServiceUnavailable, "overloaded_error", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeClaudeError(rec, tc.status, "boom")
			if rec.Code != tc.status {
				t.Fatalf("expected %d, got %d", tc.status, rec.Code)
			}

			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			// Anthropic spec: the envelope is {"type":"error","error":{...}}.
			if body["type"] != "error" {
				t.Fatalf("expected top-level type \"error\", got %v", body["type"])
			}
			errObj, _ := body["error"].(map[string]any)
			if errObj["message"] != "boom" {
				t.Fatalf("unexpected message: %v", errObj["message"])
			}
			if errObj["type"] != tc.wantType {
				t.Fatalf("unexpected error type: want %q, got %v", tc.wantType, errObj["type"])
			}
		})
	}
}

// The non-stream error shape must stay consistent with the streaming error
// frames emitted by stream_runtime_emit.go.
func TestWriteClaudeErrorMatchesStreamErrorShape(t *testing.T) {
	rec := httptest.NewRecorder()
	writeClaudeError(rec, http.StatusTooManyRequests, "slow down")

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	errObj, _ := body["error"].(map[string]any)
	if errObj["type"] != "rate_limit_error" {
		t.Fatalf("unexpected type: %v", errObj["type"])
	}
	if _, exists := errObj["code"]; exists {
		t.Fatal("OpenAI-style code field must not leak into the Anthropic error body")
	}
	if _, exists := errObj["param"]; exists {
		t.Fatal("OpenAI-style param field must not leak into the Anthropic error body")
	}
}
