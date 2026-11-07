package files

import (
	"strings"
	"testing"

	"ds2api/internal/prompt"
)

func TestTopLevelTextRefsRetainBothCaseSensitiveFileBodies(t *testing.T) {
	store := &mapContentStore{data: map[string]*storedContent{
		"file-A": {filename: "upper.txt", mimeType: "text/plain", data: []byte("UPPER_BODY")},
		"file-a": {filename: "lower.txt", mimeType: "text/plain", data: []byte("LOWER_BODY")},
	}}
	req := map[string]any{"ref_file_ids": []any{"file-A", "file-a"}, "file_ids": []any{"file-A"}, "messages": []any{map[string]any{"role": "user", "content": "TASK"}}}
	state := &expertTextInlineState{store: store, maxFileBytes: 1024}
	if err := state.inlineTopLevelRefs(req); err != nil {
		t.Fatal(err)
	}
	message := req["messages"].([]any)[0].(map[string]any)
	text := prompt.NormalizeContent(message["content"])
	if strings.Count(text, "UPPER_BODY") != 1 || strings.Count(text, "LOWER_BODY") != 1 {
		t.Fatalf("one file was dropped or duplicated: %s", text)
	}
}
