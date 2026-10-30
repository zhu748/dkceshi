package claude

import (
	"context"
	"strings"
	"testing"
)

func TestClaudeSearchPromptUsesResolvedAliasAndPreservesSystem(t *testing.T) {
	for _, resolved := range []string{"deepseek-v4.1-flash", "deepseek-v4.1-flash-search", "deepseek-v4.1-flash-search-nothinking"} {
		t.Run(resolved, func(t *testing.T) {
			store := claudeHistoryConfig{aliases: map[string]string{"client-model": resolved}, thinkingInjection: true}
			norm, err := normalizeClaudeRequest(store, map[string]any{
				"model": "client-model", "system": "CALLER-SYSTEM-RULE",
				"messages": []any{map[string]any{"role": "user", "content": "Research this topic"}},
			})
			if err != nil {
				t.Fatal(err)
			}
			h := &Handler{Store: store}
			out, err := h.applyCurrentInputFile(context.Background(), nil, norm.Standard)
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			if out.Search {
				count = 1
			}
			if strings.Count(out.FinalPrompt, "Built-in web search protocol") != count || !strings.Contains(out.FinalPrompt, "CALLER-SYSTEM-RULE") {
				t.Fatal("native search gating or caller system instruction was lost")
			}
			if out.PromptTokenText != out.FinalPrompt || len(out.ToolNames) != 0 || out.SystemTextToolsApply() {
				t.Fatal("native search altered client tool detection or token accounting")
			}
		})
	}
}
