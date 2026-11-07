package gemini

import "testing"

func TestGeminiRemoteFilesCannotBecomeUnreadableText(t *testing.T) {
	for _, key := range []string{"fileData", "file_data"} {
		for _, nested := range []bool{false, true} {
			part := map[string]any{key: map[string]any{"fileUri": "https://example.test/doc.pdf", "mimeType": "application/pdf"}}
			if nested {
				part = map[string]any{"functionResponse": map[string]any{"name": "read", "response": "OK", "parts": []any{part}}}
			}
			req := map[string]any{"contents": []any{map[string]any{"role": "user", "parts": []any{part}}}}
			if _, err := normalizeGeminiRequest(nil, "deepseek-v4.1-flash", req, false); err == nil {
				t.Fatalf("remote file accepted as text: key=%s nested=%v", key, nested)
			}
		}
	}
}
