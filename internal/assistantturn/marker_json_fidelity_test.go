package assistantturn

import (
	"testing"

	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
)

func TestFinalValidationKeepsMarkerInsideNestedJSONArgument(t *testing.T) {
	const literal = "<|Q7ZK3M|invoke>"
	raw := `<|Q7ZK3M|tool_calls><|Q7ZK3M|invoke name="write"><|Q7ZK3M|parameter name="payload">{"snippet":"` + literal + `"}</|Q7ZK3M|parameter></|Q7ZK3M|invoke></|Q7ZK3M|tool_calls>`
	tools := []any{map[string]any{"name": "write", "parameters": map[string]any{"type": "object", "required": []any{"payload"}, "properties": map[string]any{"payload": map[string]any{"type": "object", "properties": map[string]any{"snippet": map[string]any{"type": "string"}}}}}}}
	opts := BuildOptions{ToolMarker: "Q7ZK3M", ToolsRaw: tools, ToolChoice: promptcompat.DefaultToolChoicePolicy()}
	for _, turn := range []Turn{
		BuildTurnFromCollected(sse.CollectResult{Text: raw}, opts),
		BuildTurnFromStreamSnapshot(StreamSnapshot{RawText: raw, VisibleText: raw}, opts),
	} {
		if turn.Error != nil || len(turn.ToolCalls) != 1 {
			t.Fatalf("valid call rejected: %+v", turn.Error)
		}
		payload, _ := turn.ToolCalls[0].Input["payload"].(map[string]any)
		if payload["snippet"] != literal {
			t.Fatalf("validated parameter changed: %+v", payload)
		}
	}
}
