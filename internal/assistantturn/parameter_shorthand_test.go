package assistantturn

import (
	"context"
	"strings"
	"testing"

	"ds2api/internal/sse"
	"ds2api/internal/toolcall"
)

func TestFieldNamedParameterShorthandPassesSchemaLocally(t *testing.T) {
	const raw = `<|NTNAFD|tool_calls><|NTNAFD|invoke name="pwsh"><|NTNAFD|parameter name="command"><![CDATA[Get-Date]]></|NTNAFD|parameter><|NTNAFD|description"><![CDATA[Show current date]]></|NTNAFD|parameter></|NTNAFD|invoke></|NTNAFD|tool_calls>`
	opts := BuildOptions{
		ToolMarker: "NTNAFD", ToolNames: []string{"pwsh"},
		ToolsRaw: []any{map[string]any{"type": "function", "function": map[string]any{
			"name": "pwsh", "parameters": map[string]any{
				"type": "object", "required": []any{"command", "description"}, "additionalProperties": false,
				"properties": map[string]any{"command": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"}},
			},
		}}},
		ToolCallRepair: func(context.Context, string) (string, error) {
			t.Fatal("deterministic shorthand recovery must not call the repair model")
			return "", nil
		},
	}
	for _, turn := range []Turn{
		BuildTurnFromCollected(sse.CollectResult{Text: raw}, opts),
		BuildTurnFromStreamSnapshot(StreamSnapshot{RawText: raw}, opts),
	} {
		if turn.Error != nil || len(turn.ToolCalls) != 1 || turn.ToolCalls[0].Input["description"] != "Show current date" || turn.StopReason != StopReasonToolCalls {
			t.Fatalf("schema-valid shorthand failed finalization: error=%#v calls=%#v", turn.Error, turn.ToolCalls)
		}
		if err := toolcall.ValidateToolArguments(turn.ToolCalls, opts.ToolsRaw); err != nil {
			t.Fatal(err)
		}
	}
	// Recovery cannot bypass required/additionalProperties checks or invent a
	// description when no source field exists.
	for _, invalid := range []string{
		strings.Replace(raw, `<|NTNAFD|description"><![CDATA[Show current date]]></|NTNAFD|parameter>`, "", 1),
		strings.Replace(raw, `<|NTNAFD|description">`, `<|NTNAFD|unexpected">`, 1),
	} {
		for _, turn := range []Turn{
			BuildTurnFromCollected(sse.CollectResult{Text: invalid}, opts),
			BuildTurnFromStreamSnapshot(StreamSnapshot{RawText: invalid}, opts),
		} {
			if turn.Error == nil || turn.Error.Code != "invalid_tool_arguments" {
				t.Fatalf("invalid arguments bypassed schema validation: %#v", turn.Error)
			}
		}
	}
}
