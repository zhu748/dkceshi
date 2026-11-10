package assistantturn

import (
	"context"
	"reflect"
	"testing"

	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
)

func TestEntityStringsAndNestedLineEndingsReachFinalDelivery(t *testing.T) {
	for _, tc := range []struct {
		body   string
		want   any
		schema map[string]any
	}{
		{"  &lt;note&gt;hello&lt;/note&gt;\r\n", "  <note>hello</note>\r\n", map[string]any{"type": "string"}},
		{"&amp;lt;note&amp;gt;", "&lt;note&gt;", map[string]any{"type": "string"}},
		{"<text>  a\r\nb\rc\n </text>", map[string]any{"text": "  a\r\nb\rc\n "}, map[string]any{"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string"}}}},
	} {
		tools := []any{map[string]any{"name": "write", "parameters": map[string]any{"type": "object", "properties": map[string]any{"body": tc.schema}, "required": []any{"body"}}}}
		raw := `<tool_calls><invoke name="write"><parameter name="body">` + tc.body + `</parameter></invoke></tool_calls>`
		opts := BuildOptions{ToolsRaw: tools, ToolChoice: promptcompat.DefaultToolChoicePolicy()}
		for _, turn := range []Turn{BuildTurnFromCollected(sse.CollectResult{Text: raw}, opts), BuildTurnFromStreamSnapshot(StreamSnapshot{RawText: raw, VisibleText: raw}, opts)} {
			if turn.Error != nil || len(turn.ToolCalls) != 1 || !reflect.DeepEqual(turn.ToolCalls[0].Input["body"], tc.want) {
				t.Fatalf("parameter altered before delivery: error=%+v calls=%+v want=%#v", turn.Error, turn.ToolCalls, tc.want)
			}
		}
	}
}

func TestFormatRepairCannotReinterpretEntityStringAsObject(t *testing.T) {
	const source = `<|EPSE|call name="write"><|EPSE|parameter name="body">&lt;note&gt;hello&lt;/note&gt;</|EPSE|parameter></|EPSE|call>`
	tools := []any{map[string]any{"name": "write", "parameters": map[string]any{"type": "object", "properties": map[string]any{"body": map[string]any{"type": "string"}}}}}
	for _, changed := range []bool{false, true} {
		invoked := false
		opts := BuildOptions{ToolsRaw: tools, ToolCallRepair: func(context.Context, string) (string, error) {
			invoked = true
			value := `<note>hello</note>`
			if changed {
				value = `{"note":"hello"}`
			}
			return `<tool_calls><invoke name="write"><parameter name="body"><![CDATA[` + value + `]]></parameter></invoke></tool_calls>`, nil
		}}
		turn := BuildTurnFromCollected(sse.CollectResult{Text: source}, opts)
		if !invoked || changed && turn.Error == nil || !changed && (turn.Error != nil || len(turn.ToolCalls) != 1 || turn.ToolCalls[0].Input["body"] != "<note>hello</note>") {
			t.Fatalf("changed=%v invoked=%v error=%+v calls=%+v", changed, invoked, turn.Error, turn.ToolCalls)
		}
	}
}
