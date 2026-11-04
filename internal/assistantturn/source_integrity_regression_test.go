package assistantturn

import (
	"context"
	"ds2api/internal/sse"
	"ds2api/internal/toolcall"
	"strings"
	"testing"
)

func TestFormatRepairCannotDiscardUnboundSourceData(t *testing.T) {
	fixed := `<tool_calls><invoke name="check"><parameter name="mode">true</parameter></invoke></tool_calls>`
	for _, extra := range []string{`<parameter>42</parameter>`, `<invoke></invoke>`, `CRITICAL EXTRA DATA`, `"unbound string"`, `<wrong name="x">42</wrong>`, `<![CDATA[unbound]]>`, `<![CDATA[unclosed`} {
		for _, boundary := range []string{`</invoke>`, `</tool_calls>`} {
			raw := strings.Replace(fixed, boundary, extra+boundary, 1)
			turn := BuildTurnFromCollected(sse.CollectResult{Text: raw}, BuildOptions{ToolNames: []string{"check"}, ToolCallRepair: func(context.Context, string) (string, error) { return fixed, nil }})
			if turn.Error == nil || turn.Error.Code != "invalid_tool_arguments" {
				t.Fatalf("source data discarded for %q: %+v", extra, turn)
			}
		}
	}
}

func TestDuplicateToolAttributesNeverBecomeExecutable(t *testing.T) {
	for _, raw := range []string{
		`<tool_calls><invoke name="read" name="delete"><parameter name="path">a</parameter></invoke></tool_calls>`,
		`<tool_calls><invoke name="read"><parameter name="path" name="other">a</parameter></invoke></tool_calls>`,
		`<tool_calls><invoke name="read" call_id="a" call_id="b"></invoke></tool_calls>`,
		`<|EPSE|tool_calls><|EPSE|invoke name="read" name="delete"></|EPSE|invoke></|EPSE|tool_calls>`,
	} {
		turn := BuildTurnFromCollected(sse.CollectResult{Text: raw}, BuildOptions{ToolCallRepair: func(context.Context, string) (string, error) {
			return `<tool_calls><invoke name="delete"></invoke></tool_calls>`, nil
		}})
		if turn.Error == nil || len(toolcall.ParseToolCalls(raw, nil)) != 0 {
			t.Fatalf("ambiguous attributes executed: %+v", turn)
		}
	}
}
