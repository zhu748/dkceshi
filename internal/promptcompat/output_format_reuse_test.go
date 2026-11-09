package promptcompat

import (
	"strings"
	"testing"
)

func TestOutputFormatReuseMatchesFullPromptRebuild(t *testing.T) {
	for _, thinking := range []string{"", "THINKING_PRELUDE"} {
		request := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{map[string]any{"role": "system", "content": "EXACT_SYSTEM"}, map[string]any{"role": "assistant", "content": "PREFILL", "prefix": true}}, "tools": []any{map[string]any{"name": "read", "parameters": map[string]any{"type": "object"}}}}
		standard, err := NormalizeOpenAIChatRequest(nil, request, "", "PRIVATE_MARKER")
		if err != nil {
			t.Fatal(err)
		}
		standard.ThinkingInjectionPrompt = thinking
		standard.FinalPrompt, _ = standard.BuildPromptFromMessages(standard.Messages)
		for _, format := range []map[string]any{{"type": "json"}, {"type": "json_schema", "schema": map[string]any{"type": "object"}}, {"type": "text"}} {
			standard, err = ApplyOutputFormat(standard, format)
			if err != nil {
				t.Fatal(err)
			}
			rebuilt, _ := standard.BuildPromptFromMessages(standard.Messages)
			if standard.FinalPrompt != rebuilt || standard.PromptTokenText != rebuilt || !strings.HasSuffix(rebuilt, "PREFILL") {
				t.Fatal("reusing prompt changed generated instruction order or prefill")
			}
		}
	}
}
