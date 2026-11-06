package claude

import (
	"strings"
	"testing"
)

func TestClaudeThinkingWhitespaceSurvivesNormalization(t *testing.T) {
	const thinking = "    exact thinking\r\n\t"
	messages := normalizeClaudeMessages([]any{map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "thinking", "thinking": thinking}}}})
	if len(messages) != 1 || messages[0].(map[string]any)["reasoning_content"] != thinking {
		t.Fatalf("thinking was trimmed: %#v", messages)
	}
}

func TestClaudeTextPartsAndOutputSchemaSurviveNormalization(t *testing.T) {
	parts := []any{map[string]any{"type": "text", "text": "  code\n"}, map[string]any{"type": "text", "text": "\t next  "}}
	got, err := normalizeClaudeRequest(nil, map[string]any{
		"model": "deepseek-v4.1-flash", "system": parts,
		"messages":      []any{map[string]any{"role": "user", "content": parts}},
		"output_config": map[string]any{"format": map[string]any{"type": "json_schema", "schema": map[string]any{"type": "object", "required": []any{"value"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range got.Standard.Messages {
		if item.(map[string]any)["content"] != "  code\n\t next  " {
			t.Fatalf("text changed: %+v", item)
		}
	}
	if !strings.Contains(got.Standard.FinalPrompt, "Response format:") || got.Standard.OutputFormat.Validate(`{}`) == nil {
		t.Fatal("Claude schema did not join shared validation")
	}
}

func TestClaudeSystemMetadataSurvivesPromptAndHistory(t *testing.T) {
	norm, err := normalizeClaudeRequest(nil, map[string]any{"model": "deepseek-v4.1-flash",
		"system":   []any{map[string]any{"type": "text", "text": "SYSTEM_TEXT", "citations": []any{map[string]any{"title": "SYSTEM_CITATION"}}}},
		"messages": []any{map[string]any{"role": "user", "content": "hello"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(norm.Standard.FinalPrompt, "SYSTEM_CITATION") || !strings.Contains(norm.Standard.Messages[0].(map[string]any)["content"].(string), "SYSTEM_TEXT") {
		t.Fatal("system metadata disappeared during protocol conversion")
	}
}
