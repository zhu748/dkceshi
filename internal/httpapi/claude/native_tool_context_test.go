package claude

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClaudeProxyRejectsNativeToolConflictBeforeForwarding(t *testing.T) {
	capture := &openAIProxyCaptureStub{}
	h := &Handler{OpenAI: capture}
	for _, block := range []string{
		`{"type":"tool_use","name":"read","id":"a","tool_use_id":"b","input":{}}`,
		`{"type":"tool_use","name":"read","input":null}`,
		`{"type":"tool_result","tool_use_id":"a","tool_call_id":"b","content":"RESULT"}`,
		`{"type":"tool_result","tool_use_id":"a","is_error":"false","content":"RESULT"}`,
	} {
		rec := httptest.NewRecorder()
		h.Messages(rec, httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages?__stream_prepare=1", strings.NewReader(`{"model":"deepseek-v4.1-flash","messages":[{"role":"assistant","content":[`+block+`]}]}`)))
		if rec.Code != http.StatusBadRequest || capture.seenReq != nil {
			t.Fatalf("conflicting native context forwarded: %d %s", rec.Code, rec.Body.String())
		}
	}
}

func TestClaudeToolUseAndResultShareToolCallIDAlias(t *testing.T) {
	norm, err := normalizeClaudeRequest(nil, map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{
		map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "tool_use", "tool_call_id": "EXPLICIT", "name": "read", "input": map[string]any{}}}},
		map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_call_id": "EXPLICIT", "content": "RESULT"}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	call := norm.Standard.Messages[0].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)
	if call["id"] != "EXPLICIT" || norm.Standard.Messages[1].(map[string]any)["tool_call_id"] != "EXPLICIT" {
		t.Fatalf("tool call alias lost its result: %#v", norm.Standard.Messages)
	}
}
