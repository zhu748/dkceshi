package promptcompat

import (
	"strings"
	"testing"
)

func TestRolelessResponsesBodiesUseMessageValidation(t *testing.T) {
	for _, item := range []map[string]any{
		{"text": "FIRST", "content": "SECOND"},
		{"text": "FIRST", "content": nil},
		{"content": []any{map[string]any{"type": "text", "text": 42}}},
		{"content": "TEXT", "prefix": "true"},
	} {
		if _, err := NormalizeOpenAIResponsesRequest(nil, map[string]any{"model": "deepseek-v4.1-flash", "input": []any{item}}, ""); err == nil {
			t.Fatalf("untyped message bypassed validation: %#v", item)
		}
	}
}

func TestResponsesEmptyBodiesAndMetadataKeepBoundaries(t *testing.T) {
	for _, role := range []string{"user", "assistant", "system", "developer"} {
		for _, body := range []any{nil, "", "  \r\n"} {
			input := []any{map[string]any{"type": "message", "role": role, "content": body, "id": "EMPTY_ID", "name": "SOURCE_NAME"}, map[string]any{"role": "user", "content": "TASK"}}
			standard, err := NormalizeOpenAIResponsesRequest(nil, map[string]any{"model": "deepseek-v4.1-flash", "input": input}, "")
			if err != nil || len(standard.Messages) != 2 {
				t.Fatalf("empty %s boundary lost: %v %#v", role, err, standard.Messages)
			}
			for _, text := range []string{standard.FinalPrompt, BuildOpenAIHistoryTranscript(standard.Messages)} {
				if !strings.Contains(text, "EMPTY_ID") || !strings.Contains(text, "SOURCE_NAME") {
					t.Fatalf("message identity lost: %s", text)
				}
			}
		}
	}
	input := []any{map[string]any{"text": "  BODY\r\n", "content": "  BODY\r\n", "id": "ROLELESS_ID", "name": "SOURCE_NAME"}}
	standard, err := NormalizeOpenAIResponsesRequest(nil, map[string]any{"model": "deepseek-v4.1-flash", "input": input}, "")
	if err != nil || !strings.Contains(standard.FinalPrompt, "ROLELESS_ID") || strings.Count(standard.FinalPrompt, "  BODY\r\n") != 1 {
		t.Fatalf("roleless source metadata lost: %v %s", err, standard.FinalPrompt)
	}
	// An untyped business object with a non-text `text` field remains data.
	business := map[string]any{"text": map[string]any{"value": "OPAQUE_TEXT"}, "refusal": "OPAQUE_REFUSAL", "id": "BUSINESS_ID"}
	standard, err = NormalizeOpenAIResponsesRequest(nil, map[string]any{"model": "deepseek-v4.1-flash", "input": []any{business}}, "")
	if err != nil || !strings.Contains(standard.FinalPrompt, "OPAQUE_TEXT") || !strings.Contains(standard.FinalPrompt, "OPAQUE_REFUSAL") {
		t.Fatalf("business JSON reduced to message metadata: %v %s", err, standard.FinalPrompt)
	}
}
