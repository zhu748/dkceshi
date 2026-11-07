package prompt

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAttachmentContextRetainsDifferentSourceLabels(t *testing.T) {
	block := map[string]any{"type": "file", "file_id": "file", "filename": "OUTER_FILE", "name": "DISPLAY_LABEL", "file": map[string]any{"file_id": "file", "filename": "INNER_FILE", "displayName": "MEDIA_NAME"}}
	text := FileReferenceText(block)
	for _, want := range []string{"OUTER_FILE", "DISPLAY_LABEL", "INNER_FILE", "MEDIA_NAME"} {
		if !strings.Contains(text, want) {
			t.Errorf("attachment label %s disappeared: %s", want, text)
		}
	}
}

func TestAttachmentAliasMetadataScrubsBinaryAndDoesNotDuplicateOneName(t *testing.T) {
	block := map[string]any{"type": "input_file", "file_id": "file", "filename": "ONLY_NAME", "file_name": "ONLY_NAME", "file_data": "SECRET_BYTES", "metadata": map[string]any{"type": "input_audio", "input_audio": map[string]any{"data": "SECRET_AUDIO"}, "description": "KEPT"}}
	before, _ := json.Marshal(block)
	text := FileReferenceText(block)
	if strings.Count(text, "ONLY_NAME") != 1 || strings.Contains(text, "SECRET") || !strings.Contains(text, "KEPT") {
		t.Fatalf("alias retention duplicated names or leaked binary: %s", text)
	}
	after, _ := json.Marshal(block)
	if string(before) != string(after) {
		t.Fatal("metadata sanitization modified the attachment")
	}
}
