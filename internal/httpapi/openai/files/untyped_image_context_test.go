package files

import (
	"context"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
)

func TestUntypedImageLocatorsCannotDisappearDuringCleanup(t *testing.T) {
	for _, locator := range []any{"https://example.com/photo.png", map[string]any{"url": "https://example.com/photo.png"}, map[string]any{}} {
		block := map[string]any{"image_url": locator, "description": "IMAGE_DESCRIPTION"}
		req := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{map[string]any{"role": "user", "content": []any{block}}}}
		uploader := &nativeFileUploader{}
		err := PreprocessInlineFiles(context.Background(), &auth.RequestAuth{}, req, uploader, "deepseek-v4.1-flash", false)
		if err == nil || len(uploader.uploads) != 0 {
			t.Fatalf("unresolved image locator reached cleanup: err=%v", err)
		}
	}
}

func TestUntypedUploadedImagesPreserveReferenceAndContext(t *testing.T) {
	block := map[string]any{"file_id": "existing-image", "image_url": map[string]any{"url": "https://example.com/stale"}, "description": "IMAGE_DESCRIPTION"}
	req := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{map[string]any{"role": "user", "content": []any{block}}}}
	route := promptcompat.PlanImageRoute(req, nil)
	uploader := &nativeFileUploader{}
	if err := PreprocessInlineFiles(context.Background(), &auth.RequestAuth{}, req, uploader, "deepseek-v4.1-flash", false); err != nil {
		t.Fatal(err)
	}
	route.StripImages(req)
	standard, err := promptcompat.NormalizeOpenAIChatRequest(nil, req, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(uploader.uploads) != 0 || len(standard.RefFileIDs) != 1 || standard.RefFileIDs[0] != "existing-image" {
		t.Fatalf("existing reference changed: %v", standard.RefFileIDs)
	}
	for _, expected := range []string{"existing-image", "IMAGE_DESCRIPTION"} {
		if !strings.Contains(standard.FinalPrompt, expected) {
			t.Fatalf("uploaded image context lost %q", expected)
		}
	}
}
