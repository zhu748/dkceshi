package assistantturn_test

import (
	"context"
	"ds2api/internal/assistantturn"
	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
	"ds2api/internal/toolcall"
	"testing"
)

func TestRegressionLiteralCDATAWithParameterExample(t *testing.T) {
	original := `prefix </parameter><parameter name="description"><![CDATA[example`
	raw := `<tool_calls><invoke name="write"><parameter name="content"><![CDATA[` + original + `]]></parameter></invoke></tool_calls>`
	tools := []any{map[string]any{"type": "function", "function": map[string]any{"name": "write", "parameters": map[string]any{"type": "object", "properties": map[string]any{"content": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"}}, "required": []any{"content"}, "additionalProperties": false}}}}
	turn := assistantturn.BuildTurnFromCollected(sse.CollectResult{Text: raw}, assistantturn.BuildOptions{ToolsRaw: tools, ToolChoice: promptcompat.DefaultToolChoicePolicy()})
	t.Logf("error=%+v calls=%+v recovery=%v", turn.Error, turn.ToolCalls, turn.ParsedToolCalls.RecoverySteps)
	if turn.Error != nil || len(turn.ToolCalls) != 1 || turn.ToolCalls[0].Input["content"] != original || len(turn.ToolCalls[0].Input) != 1 {
		t.Fatal("valid literal CDATA was rewritten and accepted for delivery")
	}
}

func TestLiteralCDATAExampleCannotCreateCall(t *testing.T) {
	original := `prefix </parameter></invoke><invoke name="remove"><parameter name="path"><![CDATA[/tmp/example`
	raw := `<tool_calls><invoke name="write"><parameter name="content"><![CDATA[` + original + `]]></parameter></invoke></tool_calls>`
	parsed := toolcall.ParseToolCallsDetailed(raw, nil)
	t.Logf("calls=%+v recovery=%v", parsed.Calls, parsed.RecoverySteps)
	if len(parsed.Calls) != 1 || parsed.Calls[0].Input["content"] != original {
		t.Fatal("literal CDATA synthesized a sibling call")
	}
}

func TestLiteralCDATAStaysSingleCallAfterCatalogValidation(t *testing.T) {
	original := `prefix </parameter></invoke><invoke name="remove"><parameter name="path"><![CDATA[/tmp/example`
	raw := `<|EPSE|tool_calls><|EPSE|invoke name="write"><|EPSE|parameter name="content"><![CDATA[` + original + `]]></|EPSE|parameter></|EPSE|invoke></|EPSE|tool_calls>`
	tools := []any{map[string]any{"type": "function", "function": map[string]any{"name": "write", "parameters": map[string]any{"type": "object", "properties": map[string]any{"content": map[string]any{"type": "string"}}, "required": []any{"content"}, "additionalProperties": false}}}, map[string]any{"type": "function", "function": map[string]any{"name": "remove", "parameters": map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}, "required": []any{"path"}, "additionalProperties": false}}}}
	turn := assistantturn.BuildTurnFromCollected(sse.CollectResult{Text: raw}, assistantturn.BuildOptions{ToolsRaw: tools, ToolChoice: promptcompat.ToolChoicePolicy{Mode: promptcompat.ToolChoiceAuto, RestrictAllowed: true, Allowed: map[string]struct{}{"write": {}, "remove": {}}}})
	t.Logf("error=%+v calls=%+v", turn.Error, turn.ToolCalls)
	if turn.Error != nil || len(turn.ToolCalls) != 1 || turn.ToolCalls[0].Input["content"] != original {
		t.Fatal("synthetic sibling passes tool catalog and schema checks")
	}
}

func TestDuplicateArgumentsCannotBeGuessedByFormatRepair(t *testing.T) {
	for _, inner := range []string{
		`<parameter name="content">{"path":"a","path":"b"}</parameter>`,
		`{"input":{"path":"a","path":"b"}}`,
	} {
		called := false
		turn := assistantturn.BuildTurnFromCollected(sse.CollectResult{Text: `<tool_calls><invoke name="write">` + inner + `</invoke></tool_calls>`}, assistantturn.BuildOptions{
			ToolChoice: promptcompat.DefaultToolChoicePolicy(),
			ToolCallRepair: func(context.Context, string) (string, error) {
				called = true
				return `<tool_calls><invoke name="write"><parameter name="content">{"path":"b"}</parameter></invoke></tool_calls>`, nil
			},
		})
		if called || turn.Error == nil {
			t.Fatalf("format repair guessed conflicting arguments: called=%v error=%+v", called, turn.Error)
		}
	}
}

func TestDuplicateJSONExampleInCDATAKeepsLiteralBytes(t *testing.T) {
	literal := `{"path":"a","path":"b"}`
	turn := assistantturn.BuildTurnFromCollected(sse.CollectResult{Text: `<tool_calls><invoke name="write"><parameter name="content"><![CDATA[` + literal + `]]></parameter></invoke></tool_calls>`}, assistantturn.BuildOptions{ToolChoice: promptcompat.DefaultToolChoicePolicy()})
	if turn.Error != nil || len(turn.ToolCalls) != 1 || turn.ToolCalls[0].Input["content"] != literal {
		t.Fatal("literal JSON example rejected or changed")
	}
}

func TestDuplicateJSONKeysAreRejectedAtDelivery(t *testing.T) {
	raw := `<tool_calls><invoke name="write"><parameter name="content">{"path":"original.txt","path":"other.txt"}</parameter></invoke></tool_calls>`
	turn := assistantturn.BuildTurnFromCollected(sse.CollectResult{Text: raw}, assistantturn.BuildOptions{ToolChoice: promptcompat.DefaultToolChoicePolicy()})
	t.Logf("error=%+v calls=%+v", turn.Error, turn.ToolCalls)
	if turn.Error == nil && len(turn.ToolCalls) > 0 {
		t.Fatal("ambiguous duplicate JSON keys silently overwritten and delivered")
	}
}
