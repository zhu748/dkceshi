package promptcompat

import (
	"strings"
	"testing"
)

func TestReasoningAndInstructionsPreserveOriginalWhitespace(t *testing.T) {
	const first = "    first indented line\r\n\t"
	const second = "\n  second line  \n"
	const instructions = "\n    keep this indentation\n\t"
	inputs := [][]any{
		{map[string]any{"role": "assistant", "reasoning_content": first}},
		{map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "thinking", "thinking": first}}}},
		NormalizeResponsesInputAsMessages([]any{
			map[string]any{"type": "reasoning", "summary": []any{map[string]any{"type": "summary_text", "text": first}}},
			map[string]any{"type": "function_call", "name": "first", "call_id": "a", "arguments": "{}"},
			map[string]any{"type": "reasoning", "content": []any{map[string]any{"type": "reasoning_text", "text": second}}},
			map[string]any{"type": "function_call", "name": "second", "call_id": "b", "arguments": "{}"},
		}),
	}
	for _, messages := range inputs {
		for _, rendered := range []string{RenderPromptText(messages, false), BuildOpenAIHistoryTranscript(messages)} {
			if !strings.Contains(rendered, first) {
				t.Fatalf("reasoning whitespace changed: %q", rendered)
			}
		}
	}
	ordered := inputs[2]
	if len(ordered) != 2 || ordered[0].(map[string]any)["reasoning_content"] != first || ordered[1].(map[string]any)["reasoning_content"] != second {
		t.Fatalf("reasoning bytes or boundaries changed: %#v", ordered)
	}
	messages := ResponsesMessagesFromRequest(map[string]any{"instructions": instructions, "input": "question"})
	if messages[0].(map[string]any)["content"] != instructions {
		t.Fatal("Responses instructions lost whitespace")
	}
}

func TestTrailingUserSplitPreservesWhitespace(t *testing.T) {
	const current = "\n    indented user input\r\n\t"
	_, text, ok := SplitTrailingUserTurn([]any{
		map[string]any{"role": "user", "content": "previous"},
		map[string]any{"role": "user", "content": current},
	})
	if !ok || text != current {
		t.Fatalf("split changed current input: %q", text)
	}
}
