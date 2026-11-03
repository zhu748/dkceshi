package assistantturn

import (
	"context"
	"testing"

	"ds2api/internal/sse"
)

func TestMalformedRepairCannotChangeUnknownParameterValue(t *testing.T) {
	raw := `<|EPSE|call name="read"><|EPSE|parameter name="path">safe.txt</|EPSE|param></|EPSE|call>`
	reply := `<tool_calls><invoke name="read"><parameter name="path">different.txt</parameter></invoke></tool_calls>`
	tools := []any{map[string]any{"name": "read", "parameters": map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}}}}
	turn := BuildTurnFromCollected(sse.CollectResult{Text: raw}, BuildOptions{ToolsRaw: tools, ToolCallRepair: func(context.Context, string) (string, error) { return reply, nil }})
	if turn.Error == nil {
		t.Fatalf("changed parameter passed final validation: %+v", turn.ToolCalls)
	}
}

func TestXMLParameterCannotLoseTrailingSource(t *testing.T) {
	raw := `<tool_calls><invoke name="read"><parameter name="options"><label>kept</label></root><label>lost</label></parameter></invoke></tool_calls>`
	tools := []any{map[string]any{"name": "read", "parameters": map[string]any{"type": "object", "properties": map[string]any{"options": map[string]any{"type": "object"}}}}}
	turn := BuildTurnFromCollected(sse.CollectResult{Text: raw}, BuildOptions{ToolsRaw: tools})
	if turn.Error == nil {
		t.Fatalf("truncated parameter accepted: %+v", turn.ToolCalls)
	}
}

func TestOutputLimitSkipsRepairOnBothFinalizers(t *testing.T) {
	raw := `<|EPSE|broken><|EPSE|invoke name="read"></|EPSE|invoke></|EPSE|broken>`
	invoked := 0
	opts := BuildOptions{ToolCallRepair: func(context.Context, string) (string, error) { invoked++; return "", nil }}
	turns := []Turn{BuildTurnFromCollected(sse.CollectResult{Text: raw, Cut: sse.CutOutputLimit}, opts), BuildTurnFromStreamSnapshot(StreamSnapshot{RawText: raw, ContentCut: sse.CutOutputLimit}, opts)}
	for _, turn := range turns {
		if turn.Error == nil || turn.Error.Code != "output_limit_exceeded" {
			t.Fatalf("lost terminal error: %+v", turn.Error)
		}
	}
	if invoked != 0 {
		t.Fatalf("unusable output invoked repair %d times", invoked)
	}
}
