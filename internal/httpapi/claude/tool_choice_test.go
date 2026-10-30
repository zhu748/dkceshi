package claude

import (
	"ds2api/internal/config"
	"testing"
)

func TestRegressionClaudeToolChoice(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{}`)
	store := config.LoadStore()
	for _, mode := range []string{"none", "any", "tool"} {
		t.Run(mode, func(t *testing.T) {
			req := map[string]any{"model": "claude-sonnet-4-5", "messages": []any{map[string]any{"role": "user", "content": "hello"}}, "tools": []any{map[string]any{"name": "lookup", "input_schema": map[string]any{"type": "object"}}}, "tool_choice": map[string]any{"type": mode, "name": "lookup", "disable_parallel_tool_use": true}}
			got, err := normalizeClaudeRequest(store, req)
			if err != nil {
				t.Fatal(err)
			}
			p := got.Standard.ToolChoice
			if (mode == "none" && !p.IsNone()) || (mode != "none" && !p.IsRequired()) || !p.DisableParallel {
				t.Fatalf("tool_choice ignored: %#v", p)
			}
		})
	}
}
