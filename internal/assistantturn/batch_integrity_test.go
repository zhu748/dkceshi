package assistantturn

import (
	"context"
	"ds2api/internal/sse"
	"testing"
)

func TestRegressionRepairMustPreserveMixedCallOrder(t *testing.T) {
	dsml := `<DSML calls><DSML invoke name="first"></DSML invoke></DSML calls>`
	good := `<tool_calls><invoke name="second"></invoke></tool_calls>`
	turn := BuildTurnFromCollected(sse.CollectResult{Text: dsml + good}, BuildOptions{ToolNames: []string{"first", "second"}})
	if turn.Error != nil || len(turn.ToolCalls) != 2 || turn.ToolCalls[0].Name != "first" {
		t.Fatalf("source order first,second; result=%+v error=%+v", turn.ToolCalls, turn.Error)
	}
}

func TestRegressionDSMLRepairMustNotDiscardBrokenCanonicalSibling(t *testing.T) {
	good := `<DSML calls><DSML invoke name="read"></DSML invoke></DSML calls>`
	bad := `<tool_calls><invoke name="write"><param name="path">a.txt</param></invoke></tool_calls>`
	turn := BuildTurnFromCollected(sse.CollectResult{Text: good + bad}, BuildOptions{ToolNames: []string{"read", "write"}})
	if turn.Error == nil {
		t.Fatalf("broken write silently lost: calls=%+v parsed=%+v", turn.ToolCalls, turn.ParsedToolCalls)
	}
}

func TestRegressionRepairMustPreserveIdenticalDistinctCalls(t *testing.T) {
	good := `<tool_calls><invoke name="tick"></invoke></tool_calls>`
	dsml := `<DSML calls><DSML invoke name="tick"></DSML invoke></DSML calls>`
	turn := BuildTurnFromCollected(sse.CollectResult{Text: good + dsml}, BuildOptions{ToolNames: []string{"tick"}})
	if turn.Error != nil || len(turn.ToolCalls) != 2 {
		t.Fatalf("two sourced calls collapsed: calls=%+v error=%+v", turn.ToolCalls, turn.Error)
	}
}

func TestRegressionPartialWrapperMustNotDiscardDamagedSibling(t *testing.T) {
	for _, bad := range []string{`<|EPSE|call name="write"><|EPSE|parameter name="path">a.txt</|EPSE|parameter></|EPSE|call>`, `<|EPSE|invoke name="write"><|EPSE|parameter name="path">a.txt</|EPSE|parameter>`, `<|EPSE|invoke name="write"`} {
		raw := `<|EPSE|tool_calls><|EPSE|invoke name="read"></|EPSE|invoke>` + bad + `</|EPSE|tool_calls>`
		turn := BuildTurnFromCollected(sse.CollectResult{Text: raw}, BuildOptions{ToolNames: []string{"read", "write"}})
		if turn.Error == nil {
			t.Errorf("broken sibling disappeared: raw=%q calls=%+v parsed=%+v", raw, turn.ToolCalls, turn.ParsedToolCalls)
		}
	}
}

func TestRegressionUnknownParameterMarkupMustNotBeSilentlyDropped(t *testing.T) {
	raw := `<tool_calls><invoke name="write"><parameter name="path">a.txt</parameter><param name="content">important</param></invoke></tool_calls>`
	tools := []any{map[string]any{"type": "function", "function": map[string]any{"name": "write", "parameters": map[string]any{"type": "object", "required": []any{"path"}, "properties": map[string]any{"path": map[string]any{"type": "string"}, "content": map[string]any{"type": "string"}}}}}}
	turn := BuildTurnFromCollected(sse.CollectResult{Text: raw}, BuildOptions{ToolNames: []string{"write"}, ToolsRaw: tools})
	if turn.Error == nil {
		t.Fatalf("nonempty malformed optional argument lost: calls=%+v", turn.ToolCalls)
	}
}

func TestRepairOfPartialBatchPreservesAllOrderedOccurrences(t *testing.T) {
	bad := `<|EPSE|call name="tick"><|EPSE|parameter name="value"><![CDATA[same]]></|EPSE|parameter></|EPSE|call>`
	good := `<|EPSE|tool_calls><|EPSE|invoke name="tick"><|EPSE|parameter name="value"><![CDATA[same]]></|EPSE|parameter></|EPSE|invoke></|EPSE|tool_calls>`
	count := 0
	opts := BuildOptions{ToolNames: []string{"tick"}, ToolCallRepair: func(context.Context, string) (string, error) {
		count++
		return `<tool_calls><invoke name="tick"><parameter name="value"><![CDATA[DS_SLOT_0]]></parameter></invoke><invoke name="tick"><parameter name="value"><![CDATA[DS_SLOT_1]]></parameter></invoke></tool_calls>`, nil
	}}
	for _, turn := range []Turn{BuildTurnFromCollected(sse.CollectResult{Text: bad + good}, opts), BuildTurnFromStreamSnapshot(StreamSnapshot{RawText: bad + good}, opts)} {
		if turn.Error != nil || len(turn.ToolCalls) != 2 {
			t.Fatalf("ordered occurrences lost: calls=%#v error=%+v", turn.ToolCalls, turn.Error)
		}
		for _, call := range turn.ToolCalls {
			if call.Input["value"] != "same" {
				t.Fatal("argument value changed")
			}
		}
	}
	if count != 2 {
		t.Fatalf("each finalize should make at most one repair call: got %d", count)
	}
}
