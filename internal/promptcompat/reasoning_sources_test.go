package promptcompat

import (
	"strings"
	"testing"
)

func TestDistinctReasoningSourcesSurvivePromptAndHistory(t *testing.T) {
	const first, second = "  FIRST\r\n ", "\tSECOND  "
	cases := [][]any{
		{map[string]any{"role": "assistant", "reasoning_content": first, "content": []any{map[string]any{"type": "thinking", "thinking": second}}}},
		NormalizeResponsesInputAsMessages([]any{map[string]any{"type": "reasoning", "content": []any{map[string]any{"text": first}}, "summary": []any{map[string]any{"text": second}}}}),
	}
	for _, messages := range cases {
		for _, text := range []string{RenderPromptText(messages, true), BuildOpenAIHistoryTranscript(messages)} {
			if !strings.Contains(text, first) || !strings.Contains(text, second) {
				t.Fatalf("a distinct reasoning source disappeared: %q", text)
			}
		}
	}
}

func TestIdenticalReasoningRepresentationsAreNotDuplicated(t *testing.T) {
	const text = "DUPLICATE_SENTINEL"
	messages := []any{map[string]any{"role": "assistant", "reasoning_content": text, "content": []any{map[string]any{"type": "thinking", "thinking": text}}}}
	if got := RenderPromptText(messages, true); strings.Count(got, text) != 1 {
		t.Fatal("duplicate reasoning rendered twice")
	}
}
