package files

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"ds2api/internal/auth"
)

func TestRemoteImageSourcesFailExplicitlyInsteadOfDisappearing(t *testing.T) {
	for _, block := range []map[string]any{
		{"type": "image_url", "image_url": map[string]any{"url": "https://example.test/image.png"}},
		{"type": "input_image", "image_url": "https://example.test/image.png"},
		{"type": "image", "source": map[string]any{"type": "url", "url": "https://example.test/image.png"}},
	} {
		uploader := &nativeFileUploader{}
		err := PreprocessInlineFiles(context.Background(), &auth.RequestAuth{}, map[string]any{"messages": []any{map[string]any{"role": "user", "content": []any{block}}}}, uploader, "deepseek-v4.1-flash", false)
		status, message := InlineFileErrorDetails(err)
		if err == nil || status != http.StatusBadRequest || !strings.Contains(message, "data URL") || len(uploader.uploads) != 0 {
			t.Fatalf("remote source accepted or unhelpful error: status=%d message=%s err=%v", status, message, err)
		}
	}
}

func TestUploadedImageIDDoesNotRequireItsOldURL(t *testing.T) {
	_, matched, err := decodeOpenAIInlineFileBlock(map[string]any{"type": "input_image", "file_id": "file-existing", "image_url": "https://example.test/old.png"})
	if err != nil || matched {
		t.Fatal("an uploaded reference was treated as a new remote upload")
	}
}
