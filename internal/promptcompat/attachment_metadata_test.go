package promptcompat

import (
	"strings"
	"testing"

	"ds2api/internal/prompt"
)

func TestNativeAttachmentConversionKeepsContextMetadata(t *testing.T) {
	for _, original := range []map[string]any{
		{"type": "document", "title": "DOCUMENT_TITLE", "context": "CONTEXT_INSTRUCTION", "source": map[string]any{"type": "base64", "media_type": "text/plain", "data": "QUJDRA=="}},
		{"type": "image", "title": "IMAGE_TITLE", "context": "CONTEXT_INSTRUCTION", "source": map[string]any{"type": "url", "url": "https://example.com/image.png"}},
		{"inlineData": map[string]any{"mimeType": "image/png", "data": "QUJDRA==", "displayName": "MEDIA_NAME"}, "context": "CONTEXT_INSTRUCTION", "videoMetadata": map[string]any{"startOffset": "2s"}},
	} {
		normalized, ok := NormalizeAttachmentContent(original)
		if !ok {
			t.Fatalf("attachment not recognized: %#v", original)
		}
		// Simulate the post-upload reference at the shared rendering boundary.
		normalized["file_id"] = "file-reference"
		rendered := prompt.NormalizeContent([]any{normalized})
		if !strings.Contains(rendered, "CONTEXT_INSTRUCTION") {
			t.Fatalf("attachment metadata lost: %s", rendered)
		}
		if strings.Contains(rendered, "QUJDRA==") {
			t.Fatal("binary transport leaked into prompt")
		}
		if original["inlineData"] != nil && !strings.Contains(rendered, "MEDIA_NAME") {
			t.Fatalf("media display name lost: %s", rendered)
		}
		if original["context_metadata"] != nil {
			t.Fatal("attachment normalization mutated its source")
		}
	}
}
