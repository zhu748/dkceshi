package prompt

import (
	"strings"
	"testing"
)

func TestReferenceIdentityUsesDeepAliasAndPreservesAllLabels(t *testing.T) {
	block := map[string]any{"type": "input_file", "file_id": " file-A ", "filename": "outer.txt", "file": map[string]any{"file": map[string]any{"id": "file-A", "filename": "inner.txt"}}}
	text := FileReferenceText(block)
	for _, want := range []string{`"file_id":"file-A"`, "outer.txt", "inner.txt"} {
		if !strings.Contains(text, want) {
			t.Fatalf("reference identity/labels lost %s: %s", want, text)
		}
	}
	if block["file_id"] != " file-A " {
		t.Fatal("canonical identity rewrote the source")
	}
}

func TestFileReferenceIdentityDepthIsBounded(t *testing.T) {
	block := map[string]any{"type": "input_file"}
	next := block
	for i := 0; i < 129; i++ {
		child := map[string]any{}
		next["file"] = child
		next = child
	}
	if _, err := ResolveFileReferenceID(block); err == nil || FileReferenceText(block) != "" {
		t.Fatal("overdeep reference reached rendering")
	}
}
