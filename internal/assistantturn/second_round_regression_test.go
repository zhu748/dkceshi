package assistantturn

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"ds2api/internal/sse"
)

func secondAuditCollect(chunks []map[string]any, thinking bool) sse.CollectResult {
	var wire strings.Builder
	for _, chunk := range chunks {
		data, err := json.Marshal(chunk)
		if err != nil {
			panic(err)
		}
		wire.WriteString("data: " + string(data) + "\n\n")
	}
	wire.WriteString("data: [DONE]\n\n")
	return sse.CollectStream(&http.Response{Body: io.NopCloser(strings.NewReader(wire.String()))}, thinking, true)
}

func TestSecondRoundNestedSetReplay(t *testing.T) {
	call := `<tool_calls><invoke name="tick"></invoke></tool_calls>`
	got := secondAuditCollect([]map[string]any{
		{"p": "response/content", "o": "APPEND", "v": call},
		{"p": "response", "o": "BATCH", "v": []any{map[string]any{"p": "fragments/-1/content", "o": "SET", "v": call}}},
	}, false)
	turn := BuildTurnFromCollected(got, BuildOptions{})
	if turn.Error != nil || len(turn.ToolCalls) != 1 {
		t.Fatalf("replayed SET must not produce a second invocation: calls=%d error=%+v raw=%q", len(turn.ToolCalls), turn.Error, got.Text)
	}
}

func TestSecondRoundMultiFragmentReplay(t *testing.T) {
	a := `<tool_calls><invoke name="read"></invoke></tool_calls>`
	b := `<tool_calls><invoke name="write"></invoke></tool_calls>`
	chunk := map[string]any{"v": map[string]any{"response": map[string]any{"fragments": []any{
		map[string]any{"id": 1, "type": "RESPONSE", "content": a},
		map[string]any{"id": 2, "type": "RESPONSE", "content": b},
	}}}}
	got := secondAuditCollect([]map[string]any{chunk, chunk}, false)
	turn := BuildTurnFromCollected(got, BuildOptions{})
	if turn.Error != nil || len(turn.ToolCalls) != 2 {
		t.Fatalf("two-fragment snapshot replay must preserve two calls: calls=%d error=%+v raw=%q", len(turn.ToolCalls), turn.Error, got.Text)
	}
}

func TestSecondRoundSplitThinkingBoundary(t *testing.T) {
	got := secondAuditCollect([]map[string]any{
		{"p": "response/thinking_content", "v": "plan</thi"},
		{"p": "response/thinking_content", "v": "nk>actual answer"},
	}, true)
	if got.Text != "actual answer" {
		t.Fatalf("split closing boundary lost the answer: text=%q thinking=%q", got.Text, got.Thinking)
	}
}

func TestSecondRoundPatternPropertyNormalization(t *testing.T) {
	raw := `<tool_calls><invoke name="read"><parameter name="id_x">123</parameter></invoke></tool_calls>`
	for _, keyword := range []string{"properties", "patternProperties"} {
		key := "id_x"
		if keyword == "patternProperties" {
			key = "^id_"
		}
		schema := map[string]any{"type": "object", keyword: map[string]any{key: map[string]any{"type": "string"}}, "additionalProperties": false}
		turn := BuildTurnFromCollected(sse.CollectResult{Text: raw}, BuildOptions{ToolsRaw: []any{map[string]any{"name": "read", "parameters": schema}}})
		if turn.Error != nil || len(turn.ToolCalls) != 1 || turn.ToolCalls[0].Input["id_x"] != "123" {
			t.Errorf("keyword=%s rejected equivalent string constraint: calls=%+v error=%+v", keyword, turn.ToolCalls, turn.Error)
		}
	}
}

func TestSecondRoundLiteralClosingTagsSurviveContentPolicy(t *testing.T) {
	prefix := `<tool_calls><invoke name="write_file"><parameter name="content"><![CDATA[` + strings.Repeat("</parameter>\n", 10)
	suffix := `preserve this tail]]></parameter></invoke></tool_calls>`
	got := secondAuditCollect([]map[string]any{
		{"p": "response/content", "v": prefix},
		{"p": "response/content", "v": suffix},
	}, false)
	turn := BuildTurnFromCollected(got, BuildOptions{})
	if got.Cut != sse.CutNone || turn.Error != nil || len(turn.ToolCalls) != 1 {
		t.Fatalf("literal parameter content was cut before the real closing tag: cut=%s calls=%+v error=%+v", got.Cut, turn.ToolCalls, turn.Error)
	}
}
