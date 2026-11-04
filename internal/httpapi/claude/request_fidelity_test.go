package claude

import (
	"strings"
	"testing"
)

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
