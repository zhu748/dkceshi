package assistantturn

import (
	"context"
	"testing"

	"ds2api/internal/sse"
)

func TestFinalValidationRejectsConditionFlipsAfterLocalOrModelRepair(t *testing.T) {
	schema := map[string]any{"type": "object", "properties": map[string]any{"kind": map[string]any{"type": "integer"}},
		"if":   map[string]any{"properties": map[string]any{"kind": map[string]any{"type": "string"}}, "required": []any{"kind"}},
		"then": map[string]any{"required": []any{"confirmation"}}}
	tools := []any{map[string]any{"name": "example", "parameters": schema}}
	validShell := `<tool_calls><invoke name="example"><parameter name="kind">"42"</parameter></invoke></tool_calls>`
	badShell := `<|EPSE|call name="example"><|EPSE|parameter name="kind">"42"</|EPSE|parameter></|EPSE|call>`
	for _, streaming := range []bool{false, true} {
		for _, source := range []string{validShell, badShell} {
			called := false
			opts := BuildOptions{ToolsRaw: tools, ToolNames: []string{"example"}, ToolCallRepair: func(context.Context, string) (string, error) {
				called = true
				return validShell, nil
			}}
			var turn Turn
			if streaming {
				turn = BuildTurnFromStreamSnapshot(StreamSnapshot{RawText: source}, opts)
			} else {
				turn = BuildTurnFromCollected(sse.CollectResult{Text: source}, opts)
			}
			if turn.Error == nil || turn.Error.Code != "invalid_tool_arguments" || !FinalizeTurn(turn, FinalizeOptions{}).ShouldFail {
				t.Fatal("conditional constraint was bypassed at delivery", turn.Error)
			}
			if called != (source == badShell) {
				t.Fatalf("wrong format repair boundary: called=%v", called)
			}
		}
	}
}
