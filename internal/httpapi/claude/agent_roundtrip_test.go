package claude

import "testing"

func TestSameNameToolResultsRespectExplicitAndPendingIDs(t *testing.T) {
	messages := normalizeClaudeMessages([]any{
		map[string]any{"role": "assistant", "content": []any{
			map[string]any{"type": "tool_use", "name": "lookup", "input": map[string]any{"key": "a"}},
			map[string]any{"type": "tool_use", "name": "lookup", "id": "call_claude_1", "input": map[string]any{"key": "b"}},
		}},
		map[string]any{"role": "user", "content": []any{
			map[string]any{"type": "tool_result", "tool_use_id": "call_claude_1", "content": "B"},
			map[string]any{"type": "tool_result", "name": "lookup", "content": "A", "is_error": true},
		}},
	})
	if len(messages) != 4 {
		t.Fatalf("lost call/result: %#v", messages)
	}
	firstID := messages[0].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)["id"]
	last := messages[3].(map[string]any)
	if firstID == "call_claude_1" || last["tool_call_id"] != firstID || last["is_error"] != true || messages[2].(map[string]any)["name"] != "lookup" {
		t.Fatalf("roundtrip identity/status lost: %#v", messages)
	}
}
