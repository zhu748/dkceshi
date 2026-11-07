package files

import (
	"context"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
)

func TestFileURLDataUsesFileUploadAndKeepsContext(t *testing.T) {
	for _, key := range []string{"file_url", "url"} {
		block := map[string]any{"type": "input_file", key: "data:text/plain;base64,QUJDRA==", "filename": "source.txt", "description": "SOURCE_CONTEXT"}
		req := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{map[string]any{"role": "user", "content": []any{block}}}}
		uploader := &nativeFileUploader{}
		if err := PreprocessInlineFiles(context.Background(), &auth.RequestAuth{}, req, uploader, "deepseek-v4.1-flash", false); err != nil {
			t.Fatal(err)
		}
		standard, err := promptcompat.NormalizeOpenAIChatRequest(nil, req, "")
		if err != nil || len(uploader.uploads) != 1 || string(uploader.uploads[0].Data) != "ABCD" {
			t.Fatalf("data URL was not uploaded: %v %+v", err, uploader.uploads)
		}
		if strings.Contains(standard.FinalPrompt, "QUJDRA==") || !strings.Contains(standard.FinalPrompt, "SOURCE_CONTEXT") || !strings.Contains(standard.FinalPrompt, `"type":"input_file"`) {
			t.Fatalf("file was lost or classified as an image: %s", standard.FinalPrompt)
		}
	}
}

func TestRemoteFileLocatorsFailButUploadedIDsRemainUsable(t *testing.T) {
	for _, block := range []map[string]any{
		{"type": "input_file", "file_url": "https://example.test/doc.pdf"},
		{"type": "file", "url": "https://example.test/doc.pdf"},
		{"type": "document", "source": map[string]any{"type": "url", "url": "https://example.test/doc.pdf"}},
	} {
		req := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{map[string]any{"role": "user", "content": []any{block}}}}
		if _, err := promptcompat.NormalizeOpenAIChatRequest(nil, req, ""); err == nil {
			t.Fatalf("remote file became plain text: %#v", block)
		}
		uploader := &nativeFileUploader{}
		if err := PreprocessInlineFiles(context.Background(), &auth.RequestAuth{}, req, uploader, "deepseek-v4.1-flash", false); err == nil || len(uploader.uploads) != 0 {
			t.Fatalf("remote file upload proceeded: %v", err)
		}
	}
	block := map[string]any{"type": "input_file", "file_id": "file-existing", "file_url": "data:text/plain;base64,QUJDRA=="}
	standard, err := promptcompat.NormalizeOpenAIChatRequest(nil, map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{map[string]any{"role": "user", "content": []any{block}}}}, "")
	if err != nil || len(standard.RefFileIDs) != 1 || strings.Contains(standard.FinalPrompt, "QUJDRA==") {
		t.Fatalf("uploaded reference leaked stale payload: %v %s", err, standard.FinalPrompt)
	}
}
