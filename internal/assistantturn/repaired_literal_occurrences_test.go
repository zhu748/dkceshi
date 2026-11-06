package assistantturn

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"ds2api/internal/sse"
)

func TestFormatRepairPreservesRepeatedLiteralOccurrences(t *testing.T) {
	const open = "<|MISTYPED|broken>"
	const source = open + `<|MISTYPED|invoke name="write"><|MISTYPED|parameter name="labels"><![CDATA[ 1 ]]></|MISTYPED|parameter><|MISTYPED|parameter name="labels"><![CDATA[ "two" ]]></|MISTYPED|parameter></|MISTYPED|invoke></|MISTYPED|broken>`
	tools := []any{map[string]any{"name": "write", "parameters": map[string]any{"type": "object", "properties": map[string]any{"labels": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}}}}
	invocations := 0
	turn := BuildTurnFromCollected(sse.CollectResult{Text: source}, BuildOptions{
		ToolsRaw: tools,
		ToolCallRepair: func(_ context.Context, prompt string) (string, error) {
			invocations++
			start := strings.LastIndex(prompt, open)
			if start < 0 {
				t.Fatal("repair omitted the source batch")
			}
			fixed := strings.ReplaceAll(prompt[start:], "MISTYPED", "EPSE")
			fixed = strings.ReplaceAll(fixed, "<|EPSE|broken>", "<|EPSE|tool_calls>")
			return strings.ReplaceAll(fixed, "</|EPSE|broken>", "</|EPSE|tool_calls>"), nil
		},
	})
	if turn.Error != nil || len(turn.ToolCalls) != 1 || invocations != 1 {
		t.Fatalf("repair did not produce one validated batch: calls=%+v err=%+v attempts=%d", turn.ToolCalls, turn.Error, invocations)
	}
	if !reflect.DeepEqual(turn.ToolCalls[0].Input["labels"], []any{" 1 ", ` "two" `}) {
		t.Fatalf("repair changed repeated literal bytes: %#v", turn.ToolCalls[0].Input)
	}
}
