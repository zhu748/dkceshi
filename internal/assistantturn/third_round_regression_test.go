package assistantturn

import (
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
)

func TestFencedCDATAExamplesNeverBecomeCalls(t *testing.T) {
	for _, fence := range []string{"```", "~~~~", "````"} {
		for _, body := range []string{
			`<tool_calls><invoke name="write"><parameter name="content"><![CDATA[example]]></parameter></invoke></tool_calls>`,
			"<tool_calls>\n<invoke name=\"write\">\n<parameter name=\"content\"><![CDATA[example]]></parameter>\n</invoke>\n</tool_calls>",
		} {
			for _, end := range []string{"", "\n" + fence} {
				raw := "Example:\n" + fence + "xml\n" + body + end
				turn := BuildTurnFromCollected(sse.CollectResult{Text: raw}, BuildOptions{})
				if turn.Error != nil || len(turn.ToolCalls) != 0 || !strings.Contains(turn.Text, "example") {
					t.Fatalf("example executed or rejected: source=%q turn=%+v", raw, turn)
				}
			}
		}
	}
}

func TestCDATAStringsKeepJSONLookingBytes(t *testing.T) {
	for _, name := range []string{"content", "old_string", "payload", "input"} {
		for _, value := range []string{`"hello"`, `{"a": 1, "b": 2}`, "null", "true", "123", "  \r\nhello\t  "} {
			tools := []any{map[string]any{"name": "write", "parameters": map[string]any{"type": "object", "properties": map[string]any{name: map[string]any{"type": "string"}}, "required": []any{name}}}}
			raw := `<tool_calls><invoke name="write"><parameter name="` + name + `"><![CDATA[` + value + `]]></parameter></invoke></tool_calls>`
			turn := BuildTurnFromCollected(sse.CollectResult{Text: raw}, BuildOptions{ToolsRaw: tools})
			if turn.Error != nil || len(turn.ToolCalls) != 1 || turn.ToolCalls[0].Input[name] != value {
				t.Fatalf("name=%s source=%q calls=%+v error=%+v", name, value, turn.ToolCalls, turn.Error)
			}
		}
	}
}

func TestReasoningPromptEchoCannotLeakOrExecute(t *testing.T) {
	prompt := "Serialized conversation. Each message ends with [EndMessage:123]. secret generated instructions"
	turn := BuildTurnFromCollected(sse.CollectResult{Text: "answer", Thinking: prompt}, BuildOptions{Prompt: prompt})
	if turn.Error == nil || turn.Error.Code != OutputCodePromptEcho || turn.Thinking != "" || len(turn.ToolCalls) != 0 {
		t.Fatalf("echo accepted: %+v", turn)
	}
	for split := 0; split <= len(prompt); split++ {
		acc := StreamAccumulator{Prompt: prompt, ThinkingEnabled: true}
		var visible strings.Builder
		for _, part := range []string{prompt[:split], prompt[split:]} {
			out := acc.Apply(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "thinking", Text: part}}})
			for _, delta := range out.Parts {
				visible.WriteString(delta.VisibleText)
			}
		}
		if strings.Contains(visible.String(), "secret") {
			t.Fatalf("split=%d echo leaked", split)
		}
	}
}

func TestExplicitEmptyCatalogRejectsCalls(t *testing.T) {
	policy, err := promptcompat.ParseOpenAIToolPolicy(map[string]any{"tools": []any{}})
	if err != nil {
		t.Fatal(err)
	}
	turn := BuildTurnFromCollected(sse.CollectResult{Text: `<tool_calls><invoke name="write"></invoke></tool_calls>`}, BuildOptions{ToolsRaw: []any{}, ToolChoice: policy})
	if turn.Error == nil || turn.Error.Code != "tool_choice_violation" {
		t.Fatalf("empty catalog accepted: %+v", turn)
	}
}
