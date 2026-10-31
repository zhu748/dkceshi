package gemini

import (
	"context"
	"strings"
	"testing"
)

func TestGeminiSearchFlagUsesModelCapabilityWithoutPromptInjection(t *testing.T) {
	for _, model := range []string{"deepseek-v4.1-flash", "deepseek-v4.1-flash-search", "deepseek-v4.1-flash-search-nothinking"} {
		t.Run(model, func(t *testing.T) {
			store := testGeminiThinkingConfig{enabled: true}
			std, err := normalizeGeminiRequest(store, model, map[string]any{
				"systemInstruction": map[string]any{"parts": []any{map[string]any{"text": "CALLER-SYSTEM-RULE"}}},
				"contents":          []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": "Research this topic"}}}},
			}, false)
			if err != nil {
				t.Fatal(err)
			}
			h := &Handler{Store: store}
			out, err := h.applyCurrentInputFile(context.Background(), nil, std)
			if err != nil {
				t.Fatal(err)
			}
			if out.Search != strings.Contains(model, "-search") || strings.Contains(out.FinalPrompt, "Built-in web search protocol") || !strings.Contains(out.FinalPrompt, "CALLER-SYSTEM-RULE") {
				t.Fatal("native search gating or caller system instruction was lost")
			}
			if out.PromptTokenText != out.FinalPrompt || len(out.ToolNames) != 0 || out.SystemTextToolsApply() {
				t.Fatal("native search altered client tool detection or token accounting")
			}
		})
	}
}
