package claude

import (
	"ds2api/internal/assistantturn"
	"ds2api/internal/toolcall"
	"testing"
)

func TestBuildMessageResponseSkipsThinkingFallbackWhenFinalTextExists(t *testing.T) {
	resp := BuildMessageResponse(
		"msg_1",
		"claude-sonnet-4-5",
		[]any{map[string]any{"role": "user", "content": "hi"}},
		`{"tool_calls":[{"name":"search","input":{"q":"go"}}]}`,
		"normal answer",
		[]string{"search"},
	)

	if resp["stop_reason"] != "end_turn" {
		t.Fatalf("expected stop_reason=end_turn, got=%#v", resp["stop_reason"])
	}

	content, _ := resp["content"].([]map[string]any)
	foundText := false
	foundTool := false
	for _, block := range content {
		if block["type"] == "text" && block["text"] == "normal answer" {
			foundText = true
		}
		if block["type"] == "tool_use" {
			foundTool = true
		}
	}
	if !foundText {
		t.Fatalf("expected text block with finalText, got=%#v", resp["content"])
	}
	if foundTool {
		t.Fatalf("unexpected tool_use block when finalText exists, got=%#v", resp["content"])
	}
}

func TestToolCallIDsAreUniqueAcrossResponses(t *testing.T) {
	turn := assistantturn.Turn{ToolCalls: []toolcall.ParsedToolCall{{Name: "lookup", Input: map[string]any{}}}}
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		obj := BuildMessageResponseFromTurn("message", "model", turn, false)
		content := obj["content"].([]map[string]any)
		id := content[0]["id"].(string)
		if seen[id] {
			t.Fatalf("duplicate call ID %s", id)
		}
		seen[id] = true
	}
}
