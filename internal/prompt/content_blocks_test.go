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

func TestUnknownContentIsLabeledBoundedAndSanitized(t *testing.T) {
	got := NormalizeContent([]any{map[string]any{"type": "future_block", "file_data": "QUJDRA==", "payload": map[string]any{"data": strings.Repeat("secret", 1000)}, "description": strings.Repeat("中文", 3000)}})
	if !strings.Contains(got, "[Unsupported content block]") || strings.Contains(got, "secret") || strings.Contains(got, "QUJDRA==") || !utf8.ValidString(got) || len(got) > 4200 {
		t.Fatalf("unsafe unknown-block fallback: %q", got)
	}
}
