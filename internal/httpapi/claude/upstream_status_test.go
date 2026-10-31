package claude

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Regression: the initial non-200 upstream response used to be rewritten to a
// blanket 500, so an upstream 429 reached Anthropic clients as api_error (500)
// instead of rate_limit_error (429) — clients then skipped their retry logic,
// and the client-visible status diverged from the status recorded in history.
func TestClaudeStreamInitialUpstreamErrorPreservesStatus(t *testing.T) {
	cases := []struct {
		name          string
		status        int
		wantErrorType string
	}{
		{"too_many_requests", http.StatusTooManyRequests, "rate_limit_error"},
		{"forbidden", http.StatusForbidden, "permission_error"},
		{"bad_gateway", http.StatusBadGateway, "api_error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &Handler{}
			resp := &http.Response{
				StatusCode: tc.status,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("upstream said no")),
			}
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages", nil)

			h.handleClaudeStreamRealtime(rec, req, resp, "claude-sonnet-4-5",
				[]any{map[string]any{"role": "user", "content": "hi"}}, false, false, nil, nil)

			if rec.Code != tc.status {
				t.Fatalf("expected upstream status %d to be preserved, got %d body=%s", tc.status, rec.Code, rec.Body.String())
			}
			body := rec.Body.String()
			if !strings.Contains(body, tc.wantErrorType) {
				t.Fatalf("expected error type %q in body, got %s", tc.wantErrorType, body)
			}
			if !strings.Contains(body, "upstream said no") {
				t.Fatalf("expected upstream message to be forwarded, got %s", body)
			}
		})
	}
}
