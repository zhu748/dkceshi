package prompt

import (
	"strings"
	"testing"
)

func TestBusinessFieldNamesDoNotMakeTextBinary(t *testing.T) {
	text := NormalizeContent([]any{map[string]any{"type": "mcp_call", "result": map[string]any{
		"signature": "BUSINESS_SIGNATURE", "base64": "BASE64_EXPLANATION", "data": "TEXT_DATA",
	}}})
	for _, want := range []string{"BUSINESS_SIGNATURE", "BASE64_EXPLANATION", "TEXT_DATA"} {
		if !strings.Contains(text, want) {
			t.Fatalf("business context lost: %s", text)
		}
	}
	for _, block := range []map[string]any{
		{"type": "image", "base64": "OPAQUE_SENTINEL"},
		{"type": "thinking", "signature": "OPAQUE_SENTINEL"},
		{"type": "future", "thoughtSignature": "OPAQUE_SENTINEL"},
	} {
		if strings.Contains(NormalizeContent([]any{block}), "OPAQUE_SENTINEL") {
			t.Fatal("declared payload or provider signature escaped")
		}
	}
}

func TestReasoningAnnotationsStayBesideExactText(t *testing.T) {
	const body = "  reasoning\r\n\t"
	block := map[string]any{"type": "thinking", "thinking": body, "annotations": []any{map[string]any{"title": "REASONING_SOURCE"}}, "signature": "OPAQUE_SIGNATURE"}
	text := ReasoningText([]any{block})
	if !strings.HasPrefix(text, body) || !strings.Contains(text, "REASONING_SOURCE") || strings.Contains(text, "OPAQUE_SIGNATURE") {
		t.Fatalf("reasoning context changed: %s", text)
	}
}
