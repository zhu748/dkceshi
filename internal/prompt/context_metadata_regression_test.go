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
		{"type": "redacted_thinking", "thoughtSignature": "OPAQUE_SENTINEL"},
	} {
		if strings.Contains(NormalizeContent([]any{block}), "OPAQUE_SENTINEL") {
			t.Fatal("declared payload or provider signature escaped")
		}
	}
}

func TestOpaqueFieldNamesRemainBusinessDataOutsideProviderContainers(t *testing.T) {
	value := map[string]any{"type": "record", "encrypted_content": "BUSINESS_TEXT", "thought_signature": "BUSINESS_ID", "nested": map[string]any{"thoughtSignature": "NESTED_ID"}}
	for _, input := range []any{value, []any{value}, []any{map[string]any{"type": "text", "text": "body", "metadata": value}}} {
		text := NormalizeContent(input)
		for _, want := range []string{"BUSINESS_TEXT", "BUSINESS_ID", "NESTED_ID"} {
			if !strings.Contains(text, want) {
				t.Fatalf("business data lost: %s", text)
			}
		}
	}
}

func TestKnownTextBlocksStillOmitProviderStateAndReportItsPath(t *testing.T) {
	for _, kind := range []string{"text", "thinking", "reasoning"} {
		blocks := []any{map[string]any{"type": kind, "text": "BODY", "encrypted_content": "OPAQUE", "thoughtSignature": "OPAQUE"}}
		if text := NormalizeContent(blocks); strings.Contains(text, "OPAQUE") || !strings.Contains(text, "BODY") {
			t.Fatalf("provider state escaped into text: %s", text)
		}
		if paths := ContentOmittedPaths(blocks); len(paths) != 2 {
			t.Fatalf("provider omissions missing: %+v", paths)
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
