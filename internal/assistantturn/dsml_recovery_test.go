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
	missing := `<｜｜DSML｜｜ calls><｜｜DSML｜｜ invoke name="read"></｜｜DSML｜｜ invoke></｜｜DSML｜｜ calls>`
	turn := BuildTurnFromCollected(sse.CollectResult{Text: missing}, opts)
	if turn.Error == nil || turn.Error.Code != "invalid_tool_arguments" {
		t.Fatalf("required argument was invented or skipped: %#v", turn)
	}
}
