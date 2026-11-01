package claude

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTransientClaudeStreamErrorCanRecoverWithoutClientError(t *testing.T) {
	rec := httptest.NewRecorder()
	runtime := newClaudeStreamRuntime(rec, http.NewResponseController(rec), true, "model", nil, false, false, true, nil, nil, "prompt", nil)
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	h := &Handler{}
	result := h.consumeClaudeStreamAttempt(req, makeClaudeSSEHTTPResponse(`data: {"error":{"message":"服务器暂时不可用"}}`), runtime, false, true)
	if result.Terminal || !result.Retryable || runtime.finalErrorCode != "upstream_error" || strings.Contains(rec.Body.String(), "event: error") {
		t.Fatalf("transient error escaped retry: %s", rec.Body.String())
	}
	runtime.resetForRegeneration()
	result = h.consumeClaudeStreamAttempt(req, makeClaudeSSEHTTPResponse(`data: {"p":"response/content","v":"healthy answer"}`, `data: [DONE]`), runtime, false, true)
	if !result.Terminal || strings.Contains(rec.Body.String(), "event: error") || !strings.Contains(rec.Body.String(), "healthy answer") {
		t.Fatalf("recovery failed: %s", rec.Body.String())
	}
}

func TestClaudePermanentErrorAndPartialAnswerDoNotRegenerate(t *testing.T) {
	for _, partial := range []bool{false, true} {
		rec := httptest.NewRecorder()
		runtime := newClaudeStreamRuntime(rec, http.NewResponseController(rec), true, "model", nil, false, false, true, nil, nil, "prompt", nil)
		lines := []string{}
		if partial {
			lines = append(lines, `data: {"p":"response/content","v":"partial"}`)
		}
		lines = append(lines, `data: {"error":{"message":"内容超长，请删减后再试"}}`)
		result := (&Handler{}).consumeClaudeStreamAttempt(httptest.NewRequest(http.MethodPost, "/", nil), makeClaudeSSEHTTPResponse(lines...), runtime, false, true)
		if !result.Terminal || result.Retryable || !strings.Contains(rec.Body.String(), "event: error") {
			t.Fatalf("permanent error should be terminal: %s", rec.Body.String())
		}
	}
}
