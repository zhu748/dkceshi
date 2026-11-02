package assistantturn

import (
	"context"
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
)

func TestToolBodyFallbackScansPastProseAndStillValidatesArguments(t *testing.T) {
	tools := []any{map[string]any{"type": "function", "function": map[string]any{"name": "read", "parameters": map[string]any{"type": "object", "required": []any{"path"}, "properties": map[string]any{"path": map[string]any{"type": "string"}}, "additionalProperties": false}}}}
	invoke := `<invoke name="read"><parameter name="path"><![CDATA[a.txt]]></parameter></invoke>`
	custom := `<|Q7ZK3M|tool_calls><|Q7ZK3M|invoke name="read"><|Q7ZK3M|parameter name="path"><![CDATA[a.txt]]></|Q7ZK3M|parameter></|Q7ZK3M|invoke></|Q7ZK3M|tool_calls>`
	for _, wire := range []string{"<tool_calls>" + invoke + "</tool_calls>", invoke + "</tool_calls>", custom} {
		for _, prefix := range []string{"我先读取文件。\n", strings.Repeat("context ", 1000) + "\n"} {
			turn := BuildTurnFromCollected(sse.CollectResult{Text: prefix + wire}, BuildOptions{ToolNames: []string{"read"}, ToolsRaw: tools, ToolMarker: "Q7ZK3M", ToolChoice: promptcompat.DefaultToolChoicePolicy()})
			if turn.Error != nil || len(turn.ToolCalls) != 1 || turn.ToolCalls[0].Input["path"] != "a.txt" || turn.ParsedToolCalls.SourceChannel != "content" {
				t.Fatalf("leading prose hid call: error=%+v calls=%+v", turn.Error, turn.ToolCalls)
			}
		}
	}
	bad := BuildTurnFromCollected(sse.CollectResult{Text: `说明文字。 <tool_calls><invoke name="read"></invoke></tool_calls>`}, BuildOptions{ToolNames: []string{"read"}, ToolsRaw: tools})
	if bad.Error == nil || bad.Error.Code != "invalid_tool_arguments" {
		t.Fatal("fallback bypassed required-argument validation")
	}
}

func TestMalformedToolBodyAfterProseReachesFinalizeRepair(t *testing.T) {
	const raw = `先说明一下。 <|MISTYPED|broken><|MISTYPED|invoke name="read"><|MISTYPED|parameter name="path">a.txt</|MISTYPED|parameter></|MISTYPED|invoke></|MISTYPED|broken>`
	calls := 0
	turn := BuildTurnFromCollected(sse.CollectResult{Text: raw}, BuildOptions{ToolNames: []string{"read"}, ToolCallRepair: func(context.Context, string) (string, error) {
		calls++
		return `<tool_calls><invoke name="read"><parameter name="path">a.txt</parameter></invoke></tool_calls>`, nil
	}})
	if turn.Error != nil || len(turn.ToolCalls) != 1 || calls != 1 {
		t.Fatalf("repair missed non-leading candidate: %+v calls=%d", turn.Error, calls)
	}
}
