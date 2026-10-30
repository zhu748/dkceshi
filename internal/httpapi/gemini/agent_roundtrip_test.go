package gemini

import "testing"

func TestSameNameParallelResultsMatchDistinctPendingCalls(t *testing.T) {
	req := map[string]any{"contents": []any{
		map[string]any{"role": "model", "parts": []any{
			map[string]any{"functionCall": map[string]any{"name": "lookup", "args": map[string]any{"key": "a"}}},
			map[string]any{"functionCall": map[string]any{"name": "lookup", "id": "call_gemini_1", "args": map[string]any{"key": "b"}}},
		}},
		map[string]any{"role": "user", "parts": []any{
			map[string]any{"functionResponse": map[string]any{"name": "lookup", "response": "A"}},
			map[string]any{"functionResponse": map[string]any{"name": "lookup", "response": "B"}},
		}},
	}}
	messages := geminiMessagesFromRequest(req)
	if len(messages) != 4 {
		t.Fatalf("lost call/result: %#v", messages)
	}
	ids := []string{}
	for i := 0; i < 2; i++ {
		call := messages[i].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)
		id := call["id"].(string)
		ids = append(ids, id)
		if messages[i+2].(map[string]any)["tool_call_id"] != id {
			t.Fatal("parallel result matched wrong call")
		}
	}
	if ids[0] == ids[1] {
		t.Fatal("generated ID collided with explicit ID")
	}
}
