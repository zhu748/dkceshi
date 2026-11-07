package toolcall_test

import (
	"context"
	"strings"
	"testing"

	"ds2api/internal/assistantturn"
	"ds2api/internal/sse"
	"ds2api/internal/toolcall"
)

func TestUnconsumedAttributesCannotBecomeExecutableOrBeDroppedByRepair(t *testing.T) {
	for _, source := range []string{
		`<tool_calls><invoke name="read"><parameter name="path" value="TARGET.txt"></parameter></invoke></tool_calls>`,
		`<tool_calls><invoke name="read" arguments='{"path":"TARGET.txt"}'></invoke></tool_calls>`,
		`<tool_calls note="SOURCE"><invoke name="read"></invoke></tool_calls>`,
		`<tool_calls note="ONE" note="TWO"><invoke name="read"></invoke></tool_calls>`,
		`<tool_calls><invoke name="read"><parameter name="path" value TARGET></parameter></invoke></tool_calls>`,
		`<tool_calls><invoke name="read" arguments TARGET></invoke></tool_calls>`,
		`<tool_calls><invoke name="read"><parameter name="path" extra="SOURCE"><![CDATA[other]]></parameter></invoke></tool_calls>`,
		`<tool_calls><invoke arguments='name="read"'></invoke></tool_calls>`,
	} {
		for _, raw := range []string{source, strings.NewReplacer("</", "</|EPSE|", "<", "<|EPSE|").Replace(source), source + `<tool_calls><invoke name="read"></invoke></tool_calls>`} {
			called := false
			turn := assistantturn.BuildTurnFromCollected(sse.CollectResult{Text: raw}, assistantturn.BuildOptions{
				ToolsRaw: []any{map[string]any{"name": "read", "parameters": map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}}}},
				ToolCallRepair: func(context.Context, string) (string, error) {
					called = true
					return `<tool_calls><invoke name="read"></invoke></tool_calls>`, nil
				},
			})
			if turn.Error == nil || turn.Error.Code != "invalid_tool_arguments" || len(turn.ToolCalls) != 0 || called {
				t.Fatalf("attribute payload was silently discarded: raw=%s error=%+v calls=%+v repair=%v", raw, turn.Error, turn.ToolCalls, called)
			}
		}
	}
}

func TestAllowedAttributesAndLiteralAttributeExamplesRetainValues(t *testing.T) {
	raw := `<tool_calls id="batch" version="1"><invoke name="read" call_id="original"><parameter name="content"><![CDATA[<parameter value="literal">]]></parameter></invoke></tool_calls>`
	calls := toolcall.NormalizeParsedToolCallsForSchemas(toolcall.ParseToolCalls(raw, nil), nil)
	if len(calls) != 1 || calls[0].CallID != "original" || calls[0].Input["content"] != `<parameter value="literal">` {
		t.Fatalf("known identities or literal example changed: %+v", calls)
	}
}

func TestMalformedNativeAttributesCannotDisappearInFormatRepair(t *testing.T) {
	for _, attrs := range []string{`name="path" value TARGET string="true"`, `name="path" name="other" string="true"`} {
		raw := `<｜｜DSML｜｜ calls><｜｜DSML｜｜ invoke name="read"><｜｜DSML｜｜ parameter ` + attrs + `>BODY</｜｜DSML｜｜ parameter></｜｜DSML｜｜ invoke></｜｜DSML｜｜ calls>`
		called := false
		calls, ok := toolcall.RepairToolCallsWithLLM(context.Background(), raw, func(context.Context, string) (string, error) {
			called = true
			return `<tool_calls><invoke name="read"><parameter name="path"><![CDATA[BODY]]></parameter></invoke></tool_calls>`, nil
		})
		if called || ok || len(calls) != 0 {
			t.Fatal("native attribute data was dropped by format repair")
		}
	}
}
