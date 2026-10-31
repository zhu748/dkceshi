package assistantturn

import (
	"context"
	"reflect"
	"testing"

	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
)

func TestDSMLRecoveryAtSharedFinalizeAvoidsLLMAndValidatesSchema(t *testing.T) {
	raw := `<｜｜DSML｜｜ calls><｜｜DSML｜｜ invoke name="read"><｜｜DSML｜｜ parameter name="path" string="true">a.txt</｜｜DSML｜｜ parameter></｜｜DSML｜｜ invoke></｜｜DSML｜｜ calls>`
	tools := []any{map[string]any{"type": "function", "function": map[string]any{"name": "read", "parameters": map[string]any{"type": "object", "required": []any{"path"}, "properties": map[string]any{"path": map[string]any{"type": "string"}}}}}}
	opts := BuildOptions{ToolsRaw: tools, ToolChoice: promptcompat.DefaultToolChoicePolicy(), ToolCallRepair: func(context.Context, string) (string, error) {
		t.Fatal("complete native DSML must not invoke LLM repair")
		return "", nil
	}}
	collected := BuildTurnFromCollected(sse.CollectResult{Text: raw}, opts)
	streamed := BuildTurnFromStreamSnapshot(StreamSnapshot{RawText: raw}, opts)
	if collected.Error != nil || collected.Text != "" || len(collected.ToolCalls) != 1 || !reflect.DeepEqual(collected.ToolCalls, streamed.ToolCalls) || streamed.Error != nil {
		t.Fatalf("collected=%#v streamed=%#v", collected, streamed)
	}
	for _, hidden := range []Turn{
		BuildTurnFromCollected(sse.CollectResult{ToolDetectionThinking: raw}, opts),
		BuildTurnFromStreamSnapshot(StreamSnapshot{DetectionThinking: raw}, opts),
	} {
		if hidden.Error != nil || hidden.Text != "" || hidden.Thinking != "" || !reflect.DeepEqual(hidden.ToolCalls, collected.ToolCalls) {
			t.Fatalf("hidden DSML recovery changed output or exposed thinking: %#v", hidden)
		}
	}
	missing := `<｜｜DSML｜｜ calls><｜｜DSML｜｜ invoke name="read"></｜｜DSML｜｜ invoke></｜｜DSML｜｜ calls>`
	turn := BuildTurnFromCollected(sse.CollectResult{Text: missing}, opts)
	if turn.Error == nil || turn.Error.Code != "invalid_tool_arguments" {
		t.Fatalf("required argument was invented or skipped: %#v", turn)
	}
}

func TestReasoningCannotHideABrokenBodyInvocation(t *testing.T) {
	bad := `<tool_calls><invoke name="write"><param name="path">a.txt</param></invoke></tool_calls>`
	thinking := `<tool_calls><invoke name="read"></invoke></tool_calls>`
	for _, turn := range []Turn{
		BuildTurnFromCollected(sse.CollectResult{Text: bad, ToolDetectionThinking: thinking}, BuildOptions{}),
		BuildTurnFromStreamSnapshot(StreamSnapshot{RawText: bad, DetectionThinking: thinking}, BuildOptions{}),
	} {
		if turn.Error == nil || turn.Error.Code != "invalid_tool_arguments" || len(turn.ToolCalls) != 0 {
			t.Fatalf("thinking replaced an unhandled body call: %#v", turn)
		}
	}
}
