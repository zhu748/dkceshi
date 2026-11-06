package claude

import (
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
)

func TestClaudeInterleavedContentRetainsSourceOrder(t *testing.T) {
	request := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{
		map[string]any{"role": "assistant", "content": []any{
			map[string]any{"type": "text", "text": "ORDER_1"},
			map[string]any{"type": "thinking", "thinking": "ORDER_2"},
			map[string]any{"type": "text", "text": "ORDER_3"},
			map[string]any{"type": "thinking", "thinking": "ORDER_4"},
			map[string]any{"type": "tool_use", "id": "c1", "name": "read", "input": map[string]any{"path": "ORDER_5"}},
		}},
		map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": "c1", "content": "ORDER_6"}}},
	}}
	got, err := normalizeClaudeRequest(nil, request)
	if err != nil {
		t.Fatal(err)
	}
	rebuilt, _ := got.Standard.BuildPromptFromMessages(got.Standard.Messages)
	for _, rendered := range []string{got.Standard.FinalPrompt, rebuilt, promptcompat.BuildOpenAIHistoryTranscript(got.Standard.Messages)} {
		position := -1
		for _, token := range []string{"ORDER_1", "ORDER_2", "ORDER_3", "ORDER_4", "ORDER_5", "ORDER_6"} {
			next := strings.Index(rendered, token)
			if next <= position {
				t.Fatalf("%s missing or reordered: %s", token, rendered)
			}
			position = next
		}
	}
	if len(got.Standard.InputContext) == 0 {
		t.Fatal("missing protocol source fingerprints")
	}
}

func TestClaudeEmptyContentRetainsMessageBoundary(t *testing.T) {
	got := normalizeClaudeMessages([]any{map[string]any{"role": "assistant", "content": []any{}}})
	if len(got) != 1 || len(promptcompat.NormalizeOpenAIMessagesForPrompt(got, "")) != 1 {
		t.Fatal("empty native message disappeared")
	}
}
