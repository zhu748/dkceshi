package prompt

import (
	"strings"
	"testing"
)

func TestDataURLTextSurvivesButDeclaredBinaryDoesNot(t *testing.T) {
	literal := "data:text/plain;base64,QUJD"
	for _, block := range []map[string]any{
		{"type": "mcp_call", "output": literal},
		{"type": "future_block", "payload": map[string]any{"data": literal, "text": literal}},
	} {
		if text := NormalizeContent([]any{block}); !strings.Contains(text, literal) {
			t.Fatalf("business text dropped: %s", text)
		}
	}
	for _, block := range []map[string]any{
		{"type": "input_audio", "input_audio": map[string]any{"data": "BINARY_SENTINEL", "format": "wav"}},
		{"type": " INPUT_AUDIO ", "input_audio": map[string]any{"data": "BINARY_SENTINEL", "format": "wav"}},
		{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,BINARY_SENTINEL"}},
		{"type": "base64", "data": "BINARY_SENTINEL"},
	} {
		if text := NormalizeContent([]any{block}); strings.Contains(text, "BINARY_SENTINEL") || !strings.Contains(text, "omitted_binary_payload") {
			t.Fatalf("binary transport leaked: %s", text)
		}
	}
}

func TestMediaFieldNamesInsideBusinessDataDoNotRemoveText(t *testing.T) {
	const literal = "data:text/plain;base64,QUJD"
	block := map[string]any{"type": "mcp_result", "payload": map[string]any{
		"input_audio": map[string]any{"data": "TRANSCRIPT"}, "image_url": map[string]any{"url": literal},
	}}
	text := NormalizeContent([]any{block})
	if !strings.Contains(text, "TRANSCRIPT") || !strings.Contains(text, literal) {
		t.Fatal("nested business context was treated as declared media")
	}
}

func TestHistoricalArgumentUnicodeIsNeverReplaced(t *testing.T) {
	const raw = `{"value":"\ud800"}`
	text := FormatToolCallsForPrompt([]any{map[string]any{"name": "check", "arguments": raw}})
	if !strings.Contains(text, raw) || strings.Contains(text, "�") {
		t.Fatalf("historical arguments were rewritten: %s", text)
	}
}
