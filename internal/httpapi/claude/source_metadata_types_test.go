package claude

import "testing"

func TestClaudeSourceMetadataTypesAreCheckedBeforeBlockSplitting(t *testing.T) {
	for _, message := range []map[string]any{
		{"role": 123, "content": "TASK"},
		{"role": "assistant", "prefix": "true", "content": []any{map[string]any{"type": "text", "text": "PREFIX"}}},
		{"role": "user", "is_error": "false", "content": "TASK"},
		{"role": "assistant", "content": []any{map[string]any{"type": "tool_use", "id": "c", "name": 123, "input": map[string]any{}}}},
	} {
		if _, err := normalizeClaudeRequest(nil, map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{message}}); err == nil {
			t.Fatalf("invalid source metadata was flattened: %+v", message)
		}
	}
}
