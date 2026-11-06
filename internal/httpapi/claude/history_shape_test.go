package claude

import "testing"

func TestClaudeRejectsNamelessToolUseBeforePromptAssembly(t *testing.T) {
	req := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "tool_use", "id": "call1", "input": map[string]any{"text": "LOST"}}}}}}
	if _, err := normalizeClaudeRequest(nil, req); err == nil {
		t.Fatal("nameless native tool use silently disappeared")
	}
}

func TestClaudeMalformedContentCannotBeSilentlySkipped(t *testing.T) {
	for _, message := range []any{42, map[string]any{"role": "user", "content": []any{"LOST"}}, map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": 42}}}, map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "thinking", "text": 42}}}} {
		req := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{message, map[string]any{"role": "user", "content": "valid"}}}
		if _, err := normalizeClaudeRequest(nil, req); err == nil {
			t.Fatalf("invalid source disappeared: %#v", message)
		}
	}
}

func TestClaudeMalformedSystemCannotDisappear(t *testing.T) {
	for _, system := range []any{42, []any{"LOST"}, []any{map[string]any{"type": "text", "text": 42}}, []any{map[string]any{"type": "future", "text": "LOST"}}} {
		req := map[string]any{"model": "deepseek-v4.1-flash", "system": system, "messages": []any{map[string]any{"role": "user", "content": "valid"}}}
		if _, err := normalizeClaudeRequest(nil, req); err == nil {
			t.Fatal("malformed system was silently omitted")
		}
	}
}
