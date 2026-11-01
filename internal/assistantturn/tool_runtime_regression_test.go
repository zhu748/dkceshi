package assistantturn

import (
	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
	"strings"
	"testing"
)

func TestToolRuntimeRegressionToolOnlyTokenAccounting(t *testing.T) {
	raw := `<tool_calls><invoke name="write_file"><parameter name="content"><![CDATA[` + strings.Repeat("hello world ", 100) + `]]></parameter></invoke></tool_calls>`
	turn := BuildTurnFromCollected(sse.CollectResult{Text: raw}, BuildOptions{Model: "deepseek-v4.1-flash"})
	if turn.Error != nil {
		t.Fatal(turn.Error)
	}
	if turn.Usage.OutputTokens == 0 {
		t.Errorf("tool call with 1200-byte content has zero output tokens; calls=%d visible=%q", len(turn.ToolCalls), turn.Text)
	}
}

func TestToolRuntimeRegressionInvalidCatalogMustFailClosed(t *testing.T) {
	req := map[string]any{"tools": []any{map[string]any{"type": "function", "function": map[string]any{"parameters": map[string]any{"type": "object"}}}}}
	policy, err := promptcompat.ParseOpenAIToolPolicy(req)
	if err != nil {
		return
	}
	turn := BuildTurnFromCollected(sse.CollectResult{Text: `<tool_calls><invoke name="undeclared"></invoke></tool_calls>`}, BuildOptions{ToolsRaw: req["tools"], ToolChoice: policy})
	if turn.Error == nil {
		t.Errorf("invalid formal catalog accepted undeclared call; policy=%+v calls=%+v", policy, turn.ToolCalls)
	}
}

func TestToolRuntimeRegressionReferencedStringNormalization(t *testing.T) {
	raw := `<tool_calls><invoke name="read_file"><parameter name="path">123</parameter></invoke></tool_calls>`
	for _, ref := range []bool{false, true} {
		path := map[string]any{"type": "string"}
		if ref {
			path = map[string]any{"$ref": "#/$defs/Path"}
		}
		schema := map[string]any{"type": "object", "$defs": map[string]any{"Path": map[string]any{"type": "string"}}, "properties": map[string]any{"path": path}, "required": []any{"path"}}
		tools := []any{map[string]any{"name": "read_file", "parameters": schema}}
		turn := BuildTurnFromCollected(sse.CollectResult{Text: raw}, BuildOptions{ToolsRaw: tools})
		t.Logf("ref=%v calls=%+v error=%+v", ref, turn.ToolCalls, turn.Error)
		if turn.Error != nil {
			t.Errorf("equivalent string schema rejected with ref=%v", ref)
		}
	}
}

func TestToolRuntimeRegressionUpstreamToolArgumentFidelity(t *testing.T) {
	prefix := `<tool_calls><invoke name="write_file"><parameter name="content"><![CDATA[`
	suffix := `]]></parameter></invoke></tool_calls>`
	for _, tc := range []struct {
		name   string
		pieces []string
		want   string
	}{
		{"think literal one frame", []string{prefix + `before <think>literal</think> after` + suffix}, `before <think>literal</think> after`},
		{"think literal split frames", []string{prefix + `before <thi`, `nk>literal</thi`, `nk> after` + suffix}, `before <think>literal</think> after`},
		{"content filter literal split frames", []string{prefix + `before `, `CONTENT_FILTER=enabled;`, ` after` + suffix}, `before CONTENT_FILTER=enabled; after`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var collected strings.Builder
			for _, piece := range tc.pieces {
				parsed := sse.ParseDeepSeekContentChunk(map[string]any{"p": "response/content", "v": piece}, false, "text")
				for _, part := range parsed.Parts {
					collected.WriteString(part.Text)
				}
			}
			turn := BuildTurnFromCollected(sse.CollectResult{Text: collected.String()}, BuildOptions{})
			if turn.Error != nil || len(turn.ToolCalls) != 1 {
				t.Fatalf("error=%+v calls=%+v", turn.Error, turn.ToolCalls)
			}
			got := turn.ToolCalls[0].Input["content"]
			if got != tc.want {
				t.Errorf("tool executes silently changed argument: got=%q want=%q", got, tc.want)
			}
		})
	}
}

func TestToolRuntimeRegressionFilteredBatchMustRemainAtomic(t *testing.T) {
	raw := `<tool_calls><invoke name="read"></invoke><invoke name="write"><param name="path">lost.txt</param></invoke></tool_calls>`
	tools := []any{map[string]any{"name": "read", "parameters": map[string]any{"type": "object"}}, map[string]any{"name": "write", "parameters": map[string]any{"type": "object"}}}
	policy, err := promptcompat.ParseOpenAIToolPolicy(map[string]any{"tools": tools})
	if err != nil {
		t.Fatal(err)
	}
	turn := BuildTurnFromCollected(sse.CollectResult{Text: raw, ContentFilter: true}, BuildOptions{ToolsRaw: tools, ToolChoice: policy})
	if turn.Error == nil {
		t.Errorf("formal catalog with filtered broken batch accepted partial calls: %+v", turn.ToolCalls)
	}
}
