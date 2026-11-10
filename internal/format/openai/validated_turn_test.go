package openai

import (
	"strings"
	"testing"

	"ds2api/internal/assistantturn"
	"ds2api/internal/sse"
)

func TestValidatedTurnArgumentsRemainIdenticalAcrossRenderers(t *testing.T) {
	tools := []any{map[string]any{"name": "write", "parameters": map[string]any{"type": "object", "properties": map[string]any{"body": map[string]any{"type": "string"}, "sequence": map[string]any{"type": "integer"}}, "required": []any{"body", "sequence"}}}}
	raw := `<tool_calls><invoke name="write"><parameter name="body">&lt;note&gt;hello&lt;/note&gt;</parameter><parameter name="sequence">9007199254740993</parameter></invoke></tool_calls>`
	turn := assistantturn.BuildTurnFromCollected(sse.CollectResult{Text: raw}, assistantturn.BuildOptions{ToolsRaw: tools})
	if turn.Error != nil || len(turn.ToolCalls) != 1 {
		t.Fatal("shared validation failed", turn.Error)
	}
	chat := BuildChatCompletionFromTurn("id", "model", turn)
	message := chat["choices"].([]map[string]any)[0]["message"].(map[string]any)
	chatArgs := message["tool_calls"].([]map[string]any)[0]["function"].(map[string]any)["arguments"].(string)
	responses := BuildResponseObjectFromTurn("id", "model", turn, tools)
	responseArgs := responses["output"].([]any)[0].(map[string]any)["arguments"].(string)
	streamArgs := FormatValidatedChatToolCalls(turn.ToolCalls, tools)[0]["function"].(map[string]any)["arguments"].(string)
	if chatArgs != responseArgs || chatArgs != streamArgs || !strings.Contains(chatArgs, "9007199254740993") || !strings.Contains(chatArgs, `\u003cnote\u003ehello\u003c/note\u003e`) {
		t.Fatalf("validated values changed at rendering boundary: chat=%s responses=%s stream=%s", chatArgs, responseArgs, streamArgs)
	}
}
