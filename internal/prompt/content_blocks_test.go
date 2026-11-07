package prompt

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestContentBlocksPreserveExactTextAndRefusals(t *testing.T) {
	got := NormalizeContent([]any{
		map[string]any{"type": "input_text", "text": "  code\n"},
		map[string]any{"type": "input_text", "text": "\t next  "},
	})
	if got != "  code\n\t next  " {
		t.Fatalf("text parts changed: %q", got)
	}
	refusal := strings.Repeat("不能执行这个操作。", 2000)
	if !strings.Contains(NormalizeContent([]any{map[string]any{"type": "refusal", "refusal": refusal}}), refusal) {
		t.Fatal("known refusal content was lost or truncated")
	}
}

func TestOpaqueThinkingAndExplicitBinaryRemainOmitted(t *testing.T) {
	for _, block := range []map[string]any{
		{"type": "redacted_thinking", "data": "OPAQUE_SENTINEL"},
		{"type": "future_media", "encoding": "base64", "data": "OPAQUE_SENTINEL"},
	} {
		text := NormalizeContent([]any{block})
		if strings.Contains(text, "OPAQUE_SENTINEL") || !strings.Contains(text, "omitted_binary_payload") {
			t.Fatal("opaque payload was rendered as text")
		}
	}
}

func TestUnknownContentPreservesLongTextAndSanitizesBinary(t *testing.T) {
	got := NormalizeContent([]any{map[string]any{"type": "future_block", "media": map[string]any{"type": "file", "file_data": "QUJDRA=="}, "payload": map[string]any{"data": strings.Repeat("secret", 1000)}, "description": strings.Repeat("中文", 3000)}})
	if !strings.Contains(got, "[Unsupported content block]") || !strings.Contains(got, strings.Repeat("secret", 1000)) || strings.Contains(got, "QUJDRA==") || !utf8.ValidString(got) || !strings.Contains(got, strings.Repeat("中文", 3000)) {
		t.Fatalf("unsafe unknown-block fallback: %q", got)
	}
}
