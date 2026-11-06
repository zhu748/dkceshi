package prompt

import (
	"strings"
	"testing"
)

func TestRefusalBlockKeepsContextMetadata(t *testing.T) {
	text := NormalizeContent([]any{map[string]any{"type": "refusal", "refusal": " BODY ", "annotations": []any{"SOURCE_SENTINEL"}}})
	if !strings.Contains(text, " BODY ") || !strings.Contains(text, "SOURCE_SENTINEL") {
		t.Fatalf("refusal context lost: %s", text)
	}
}
