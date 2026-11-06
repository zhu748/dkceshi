package promptcompat

import (
	"strings"
	"testing"
)

func TestTextAliasesRejectConflictAndKeepMatchingWhitespace(t *testing.T) {
	for _, kind := range []string{"text", "input_text", "output_text", "thinking", "reasoning"} {
		block := map[string]any{"type": kind, "text": "  original\r\n", "content": "different"}
		req := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{map[string]any{"role": "assistant", "content": []any{block}}}}
		if _, err := NormalizeOpenAIChatRequest(nil, req, ""); err == nil {
			t.Fatalf("%s silently selected a conflicting field", kind)
		}
		if _, err := NormalizeOpenAIResponsesRequest(nil, req, ""); err == nil {
			t.Fatalf("Responses %s silently selected a conflicting field", kind)
		}
		block["content"] = block["text"]
		std, err := NormalizeOpenAIChatRequest(nil, req, "")
		if err != nil || !strings.Contains(std.FinalPrompt, "  original\r\n") {
			t.Fatalf("matching text aliases rejected or trimmed: %v", err)
		}
	}
}

func TestReasoningMetadataReachesPromptHistoryAndResponses(t *testing.T) {
	for _, req := range []map[string]any{
		{"messages": []any{map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "reasoning", "text": "reasoning", "annotations": []any{"SOURCE_SENTINEL"}}}}}},
		{"input": []any{map[string]any{"type": "reasoning", "text": "TOP_LEVEL_REASONING", "annotations": []any{"ITEM_ANNOTATION"}, "summary": []any{map[string]any{"type": "summary_text", "text": "reasoning", "annotations": []any{"SOURCE_SENTINEL"}}}}}},
	} {
		req["model"] = "deepseek-v4.1-flash"
		std, err := NormalizeOpenAIResponsesRequest(nil, req, "")
		if err != nil {
			t.Fatal(err)
		}
		for _, text := range []string{std.FinalPrompt, BuildOpenAIHistoryTranscript(std.Messages)} {
			if !strings.Contains(text, "SOURCE_SENTINEL") {
				t.Fatalf("reasoning annotation omitted: %s", text)
			}
			if req["input"] != nil && (!strings.Contains(text, "TOP_LEVEL_REASONING") || !strings.Contains(text, "ITEM_ANNOTATION")) {
				t.Fatal("Responses reasoning fields were silently preferred or dropped")
			}
		}
	}
}
