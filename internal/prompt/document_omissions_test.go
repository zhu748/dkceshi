package prompt

import (
	"reflect"
	"strings"
	"testing"
)

func TestReferencedTextDocumentRetainsTextAndReportsActualBinaryOmission(t *testing.T) {
	content := []any{map[string]any{
		"type": "document", "file_id": "file-original",
		"source":   map[string]any{"type": "text", "data": "  EXACT_TEXT\r\n"},
		"metadata": map[string]any{"media": map[string]any{"type": "file", "data": "BINARY_SENTINEL"}},
	}}
	text := NormalizeContent(content)
	if !strings.Contains(text, `"data":"  EXACT_TEXT\r\n"`) || strings.Contains(text, "BINARY_SENTINEL") {
		t.Fatalf("text and binary boundaries changed: %s", text)
	}
	if got := ContentOmittedPaths(content); !reflect.DeepEqual(got, []string{"/0/metadata/media/data"}) {
		t.Fatalf("wrong omission evidence: %v", got)
	}
}
