package chat

import (
	"ds2api/internal/promptcompat"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSchemaFailureDefersAndDiscardsInvalidAttempt(t *testing.T) {
	tools := []any{map[string]any{"type": "function", "function": map[string]any{"name": "lookup", "parameters": map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}, "required": []any{"query"}}}}}
	rec := httptest.NewRecorder()
	runtime := newChatStreamRuntime(rec, http.NewResponseController(rec), true, "id", 1, "model", "prompt", false, false, true, []string{"lookup"}, tools, promptcompat.DefaultToolChoicePolicy(), true, true)
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	content := func(raw string) *http.Response {
		payload, err := json.Marshal(map[string]any{"p": "response/content", "v": raw})
		if err != nil {
			t.Fatal(err)
		}
		return makeOpenAISSEHTTPResponse("data: "+string(payload), "data: [DONE]")
	}
	resp := content(`<tool_calls><invoke name="lookup"></invoke></tool_calls>`)
	result := (&Handler{}).consumeChatStreamAttempt(req, resp, runtime, "text", false, nil, true)
	if result.Terminal || !result.Retryable || runtime.finalErrorCode != "invalid_tool_arguments" {
		t.Fatalf("schema failure was not deferred: %+v code=%s", result, runtime.finalErrorCode)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("invalid tool attempt reached client: %s", rec.Body.String())
	}
	runtime.resetForRegeneration()
	resp = content(`<tool_calls><invoke name="lookup"><parameter name="query">ok</parameter></invoke></tool_calls>`)
	result = (&Handler{}).consumeChatStreamAttempt(req, resp, runtime, "text", false, nil, true)
	body := rec.Body.String()
	if !result.Terminal || strings.Contains(body, `"error"`) || !strings.Contains(body, `"finish_reason":"tool_calls"`) {
		t.Fatalf("valid retry failed: %+v body=%s", result, body)
	}
}

func TestTransientStreamErrorDefersButDeliveredAnswerDoesNot(t *testing.T) {
	for _, partial := range []bool{false, true} {
		var tools any
		rec := httptest.NewRecorder()
		runtime := newChatStreamRuntime(rec, http.NewResponseController(rec), true, "id", 1, "model", "prompt", false, false, true, []string{"lookup"}, tools, promptcompat.DefaultToolChoicePolicy(), true, true)
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		lines := []string{}
		if partial {
			lines = append(lines, `data: {"p":"response/content","v":"partial answer"}`)
		}
		lines = append(lines, `data: {"error":{"message":"服务器暂时不可用"}}`)
		resp := makeOpenAISSEHTTPResponse(lines...)
		result := (&Handler{}).consumeChatStreamAttempt(req, resp, runtime, "text", false, nil, true)
		if result.Terminal != partial || result.Retryable == partial {
			t.Fatalf("delivery guard failed: partial=%t result=%+v body=%s", partial, result, rec.Body.String())
		}
	}
}
