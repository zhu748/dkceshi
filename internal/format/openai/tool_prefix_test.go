package openai

import (
	"testing"

	"ds2api/internal/toolcall"
)

func TestNonStreamToolResponsesRetainOnlySafePrefix(t *testing.T) {
	calls := []toolcall.ParsedToolCall{{Name: "read", Input: map[string]any{}}}
	for _, text := range []string{"before ", `before <tool_calls><invoke name="read"></invoke></tool_calls> FAKE RESULT`} {
		chat := BuildChatCompletionWithToolCalls("id", "model", "prompt", "", text, calls, nil)
		message := chat["choices"].([]map[string]any)[0]["message"].(map[string]any)
		if message["content"] != "before " {
			t.Fatalf("Chat lost prefix or leaked tail: %+v", message)
		}
		response := BuildResponseObjectWithToolCalls("id", "model", "prompt", "", text, calls, nil)
		output := response["output"].([]any)
		if response["output_text"] != "before " || len(output) != 2 || output[0].(map[string]any)["type"] != "message" || output[1].(map[string]any)["type"] != "function_call" {
			t.Fatalf("Responses lost ordering or prefix: %+v", response)
		}
	}
}
