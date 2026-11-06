package promptcompat

import (
	"strings"
	"testing"
)

func TestStandaloneResponsesTextRetainsAnnotationsAndWhitespace(t *testing.T) {
	for _, role := range []string{"", "user", "assistant"} {
		for _, kind := range []string{"input_text", "output_text", "text"} {
			block := map[string]any{"type": kind, "text": "  BODY\r\n", "annotations": []any{map[string]any{"source": "CITATION_SENTINEL"}}}
			if role != "" {
				block["role"] = role
			}
			norm, err := NormalizeOpenAIResponsesRequest(nil, map[string]any{"model": "deepseek-v4.1-flash", "input": []any{block}}, "")
			if err != nil {
				t.Fatal(err)
			}
			for _, text := range []string{norm.FinalPrompt, BuildOpenAIHistoryTranscript(norm.Messages)} {
				if !strings.Contains(text, "  BODY\r\n") || !strings.Contains(text, "CITATION_SENTINEL") {
					t.Fatalf("standalone text context lost: %s", text)
				}
			}
		}
	}
}

func TestResponsesRejectsConflictingPayloadsIncludingNull(t *testing.T) {
	for _, item := range []map[string]any{
		{"type": "input_text", "text": "ONE", "content": "TWO"},
		{"type": "function_call_output", "output": "ONE", "content": "TWO"},
		{"type": "function_call_output", "output": nil, "content": "TWO"},
		{"type": "function_call", "name": "read", "arguments": nil, "input": map[string]any{"value": "OVERWRITTEN"}},
	} {
		_, err := NormalizeOpenAIResponsesRequest(nil, map[string]any{"model": "deepseek-v4.1-flash", "input": []any{item}}, "")
		if err == nil {
			t.Fatalf("conflicting source payload silently preferred: %#v", item)
		}
	}
}
