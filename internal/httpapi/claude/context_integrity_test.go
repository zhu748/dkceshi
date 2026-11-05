package claude

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"ds2api/internal/prompt"
	"ds2api/internal/promptcompat"
)

func TestClaudeKnownContextRetainsExactNestedText(t *testing.T) {
	body := "  DOCUMENT_HEAD\n" + strings.Repeat("中文 code {data: value}\n", 600) + "DOCUMENT_TAIL\n\t "
	for _, block := range []map[string]any{
		{"type": "document", "title": "source", "source": map[string]any{"type": "text", "media_type": "text/plain", "data": body}},
		{"type": "search_result", "title": "RAG", "source": "https://example.test/source", "content": []any{map[string]any{"type": "text", "text": body}}},
		{"type": "document", "source": map[string]any{"type": "content", "content": []any{map[string]any{"type": "text", "text": body}}}},
	} {
		original, _ := json.Marshal(block)
		preserved := preserveClaudeToolResultContent(block)
		if !reflect.DeepEqual(preserved, block) {
			t.Fatalf("known context changed: %s", original)
		}
		for _, toolResult := range []bool{false, true} {
			var content any = block
			if toolResult {
				content = map[string]any{"type": "tool_result", "tool_use_id": "call1", "content": []any{block}}
			}
			messages := normalizeClaudeMessages([]any{map[string]any{"role": "user", "content": []any{content}}})
			for _, text := range []string{promptcompat.RenderPromptText(messages, false), promptcompat.BuildOpenAIHistoryTranscript(messages)} {
				if !strings.Contains(text, "DOCUMENT_HEAD") || !strings.Contains(text, "DOCUMENT_TAIL") || strings.Contains(text, "truncated") || strings.Contains(text, "omitted_binary_payload") {
					t.Fatalf("known context lost in transcript, tool=%v", toolResult)
				}
			}
		}
		after, _ := json.Marshal(block)
		if string(after) != string(original) {
			t.Fatal("normalization mutated caller context")
		}
	}
}

func TestClaudeMixedToolAttachmentsPreserveTextAndCleanUnknowns(t *testing.T) {
	body := "  " + strings.Repeat("A", 2000) + "  "
	blocks := []any{
		map[string]any{"type": "document", "source": map[string]any{"type": "text", "media_type": "text/plain", "data": body}},
		map[string]any{"type": "input_image", "file_id": "file-screenshot"},
		map[string]any{"type": "future", "data": "PRIVATE_BINARY"},
	}
	result := normalizeClaudeToolResultContent(blocks).([]any)
	if result[0].(map[string]any)["source"].(map[string]any)["data"] != body {
		t.Fatal("text document changed beside an attachment")
	}
	text := prompt.NormalizeContent(result)
	if !strings.Contains(text, body) || !strings.Contains(text, "file-screenshot") || strings.Contains(text, "PRIVATE_BINARY") {
		t.Fatal("mixed tool content was lost or leaked binary data")
	}
}
