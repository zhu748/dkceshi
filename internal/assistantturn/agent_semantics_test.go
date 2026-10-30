package assistantturn

import (
	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
	"ds2api/internal/toolcall"
	"testing"
)

func TestRegressionLiteralToolRoleInsideToolArgument(t *testing.T) {
	literal := "before [Tool]: after"
	markup := `<|EPSE|tool_calls><|EPSE|invoke name="write_file"><|EPSE|parameter name="content"><![CDATA[` + literal + `]]></|EPSE|parameter></|EPSE|invoke></|EPSE|tool_calls>`
	turn := BuildTurnFromCollected(sse.CollectResult{Text: markup}, BuildOptions{ToolNames: []string{"write_file"}})
	if len(turn.ToolCalls) != 1 || turn.ToolCalls[0].Input["content"] != literal {
		t.Fatalf("literal content broke tool call: calls=%#v raw=%q", turn.ToolCalls, turn.RawText)
	}
}
func TestRegressionAutoRejectsUndeclaredTool(t *testing.T) {
	err := ValidateTurn(Turn{ToolCalls: []toolcall.ParsedToolCall{{Name: "undeclared", Input: map[string]any{}}}}, promptcompat.ToolChoicePolicy{Mode: promptcompat.ToolChoiceAuto, Allowed: map[string]struct{}{"lookup": {}}})
	if err == nil {
		t.Fatal("undeclared tool accepted under auto despite declared Allowed set")
	}
}
func TestRegressionStrictSchemaRequiresPath(t *testing.T) {
	tools := []any{map[string]any{"type": "function", "function": map[string]any{"name": "write_file", "strict": true, "parameters": map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}, "required": []any{"path"}, "additionalProperties": false}}}}
	turn := BuildTurnFromCollected(sse.CollectResult{Text: `<tool_calls><invoke name="write_file"></invoke></tool_calls>`}, BuildOptions{ToolsRaw: tools, ToolNames: []string{"write_file"}, ToolChoice: promptcompat.ToolChoicePolicy{Mode: promptcompat.ToolChoiceRequired}})
	if turn.Error == nil {
		t.Fatalf("strict call with missing path accepted: %#v", turn.ToolCalls)
	}
}
