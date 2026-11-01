package gemini

import "testing"

func TestToolRuntimeRegressionOutOfOrderResultCallIDAlias(t *testing.T) {
	req := map[string]any{"contents": []any{
		map[string]any{"role": "model", "parts": []any{
			map[string]any{"functionCall": map[string]any{"name": "lookup", "call_id": "a", "args": map[string]any{"key": "a"}}},
			map[string]any{"functionCall": map[string]any{"name": "lookup", "call_id": "b", "args": map[string]any{"key": "b"}}},
		}},
		map[string]any{"role": "user", "parts": []any{
			map[string]any{"functionResponse": map[string]any{"name": "lookup", "call_id": "b", "response": "result-b"}},
			map[string]any{"functionResponse": map[string]any{"name": "lookup", "call_id": "a", "response": "result-a"}},
		}},
	}}
	messages := geminiMessagesFromRequest(req)
	for _, raw := range messages {
		msg := raw.(map[string]any)
		if msg["role"] == "tool" {
			t.Logf("result=%s linked=%s", msg["content"], msg["tool_call_id"])
			if msg["content"] == "result-b" && msg["tool_call_id"] != "b" {
				t.Error("explicit out-of-order call_id result mapped to wrong call")
			}
		}
	}
}
