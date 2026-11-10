package gemini

import (
	"strings"
	"testing"
)

func TestGeminiCatalogEnvelopeAndDeclarationContextReachText(t *testing.T) {
	req := map[string]any{"contents": []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": "TASK"}}}}, "tools": []any{map[string]any{
		"usage_policy": "ENVELOPE_RULE", "functionDeclarations": []any{map[string]any{"name": "read", "usage_policy": "DECLARATION_RULE", "parameters": map[string]any{"type": "OBJECT"}}},
	}}}
	std, err := normalizeGeminiRequest(nil, "deepseek-v4.1-flash", req, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ENVELOPE_RULE", "DECLARATION_RULE"} {
		if strings.Count(std.FinalPrompt, want) != 1 {
			t.Fatalf("catalog context lost or duplicated: %s", want)
		}
	}
}
