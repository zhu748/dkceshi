package assistantturn

import (
	"context"
	"testing"

	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
	"ds2api/internal/toolcall"
)

func TestUnwrappedStringRetainsWhitespaceAtFinalDelivery(t *testing.T) {
	tools := []any{map[string]any{"name": "Edit", "parameters": map[string]any{"type": "object", "properties": map[string]any{"old_string": map[string]any{"type": "string"}}, "required": []any{"old_string"}}}}
	for _, value := range []string{"  exact line\r\n    ", " \r\n\t ", "", "  a &amp; b  "} {
		raw := `<tool_calls><invoke name="Edit"><parameter name="old_string">` + value + `</parameter></invoke></tool_calls>`
		want := value
		if value == "  a &amp; b  " {
			want = "  a & b  "
		}
		options := BuildOptions{ToolsRaw: tools, ToolChoice: promptcompat.DefaultToolChoicePolicy()}
		for _, turn := range []Turn{BuildTurnFromCollected(sse.CollectResult{Text: raw}, options), BuildTurnFromStreamSnapshot(StreamSnapshot{RawText: raw, VisibleText: raw}, options)} {
			if turn.Error != nil || len(turn.ToolCalls) != 1 || turn.ToolCalls[0].Input["old_string"] != want {
				t.Fatalf("raw string altered: %+v %+v", turn.Error, turn.ToolCalls)
			}
		}
	}
}

func TestFormatRepairCannotTrimUnwrappedString(t *testing.T) {
	const source = `<|EPSE|call name="Edit"><|EPSE|parameter name="old_string">  exact line  </|EPSE|parameter></|EPSE|call>`
	for _, value := range []string{"  exact line  ", "exact line"} {
		invoke := func(context.Context, string) (string, error) {
			return `<|EPSE|tool_calls><|EPSE|invoke name="Edit"><|EPSE|parameter name="old_string"><![CDATA[` + value + `]]></|EPSE|parameter></|EPSE|invoke></|EPSE|tool_calls>`, nil
		}
		calls, ok := toolcall.RepairToolCallsWithLLM(context.Background(), source, invoke)
		if value == "exact line" && ok {
			t.Fatal("format repair changed string bytes")
		}
		if value != "exact line" && (!ok || len(calls) != 1 || calls[0].Input["old_string"] != value) {
			t.Fatal("format-only repair rejected exact bytes", calls)
		}
	}
}
