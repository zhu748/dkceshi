package gemini

import (
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
)

func TestGeminiInterleavedContentRetainsSourceOrder(t *testing.T) {
	request := map[string]any{"contents": []any{
		map[string]any{"role": "model", "parts": []any{
			map[string]any{"text": "ORDER_1"},
			map[string]any{"thought": true, "text": "ORDER_2"},
			map[string]any{"text": "ORDER_3"},
			map[string]any{"thought": true, "text": "ORDER_4"},
			map[string]any{"functionCall": map[string]any{"id": "c1", "name": "read", "args": map[string]any{"path": "ORDER_5"}}},
		}},
		map[string]any{"role": "user", "parts": []any{map[string]any{"functionResponse": map[string]any{"id": "c1", "name": "read", "response": map[string]any{"body": "ORDER_6"}}}}},
	}}
	got, err := normalizeGeminiRequest(nil, "deepseek-v4.1-flash", request, false)
	if err != nil {
		t.Fatal(err)
	}
	rebuilt, _ := got.BuildPromptFromMessages(got.Messages)
	for _, rendered := range []string{got.FinalPrompt, rebuilt, promptcompat.BuildOpenAIHistoryTranscript(got.Messages)} {
		position := -1
		for _, token := range []string{"ORDER_1", "ORDER_2", "ORDER_3", "ORDER_4", "ORDER_5", "ORDER_6"} {
			next := strings.Index(rendered, token)
			if next <= position {
				t.Fatalf("%s missing or reordered: %s", token, rendered)
			}
			position = next
		}
	}
	if len(got.InputContext) == 0 {
		t.Fatal("missing protocol source fingerprints")
	}
}

func TestGeminiEmptyContentRetainsMessageBoundary(t *testing.T) {
	got := geminiMessagesFromRequest(map[string]any{"contents": []any{map[string]any{"role": "model", "parts": []any{}}}})
	if len(got) != 1 || len(promptcompat.NormalizeOpenAIMessagesForPrompt(got, "")) != 1 {
		t.Fatal("empty native message disappeared")
	}
}
