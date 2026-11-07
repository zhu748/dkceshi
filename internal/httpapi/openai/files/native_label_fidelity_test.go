package files

import (
	"context"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
)

func TestNativeAttachmentLabelsSurviveBothUploadOrders(t *testing.T) {
	for _, convertFirst := range []bool{false, true} {
		for _, native := range []map[string]any{
			{"type": "document", "display_name": "DISPLAY_SENTINEL", "source": map[string]any{"type": "base64", "media_type": "text/plain", "data": "QUJDRA==", "file_name": "SOURCE_SENTINEL.txt"}},
			{"inlineData": map[string]any{"mimeType": "text/plain", "data": "QUJDRA==", "filename": "SOURCE_SENTINEL.txt"}, "displayName": "DISPLAY_SENTINEL"},
		} {
			block := native
			if convertFirst {
				var ok bool
				block, ok = promptcompat.NormalizeAttachmentContent(native)
				if !ok {
					t.Fatal("native conversion failed")
				}
			}
			req := map[string]any{"messages": []any{map[string]any{"role": "user", "content": []any{block}}}}
			uploader := &nativeFileUploader{}
			if err := PreprocessInlineFiles(context.Background(), &auth.RequestAuth{}, req, uploader, "deepseek-v4.1-flash", false); err != nil {
				t.Fatal(err)
			}
			if len(uploader.uploads) != 1 || uploader.uploads[0].Filename != "SOURCE_SENTINEL.txt" || string(uploader.uploads[0].Data) != "ABCD" {
				t.Fatalf("upload metadata changed: %+v", uploader.uploads)
			}
			for _, text := range []string{promptcompat.RenderPromptText(req["messages"].([]any), false), promptcompat.BuildOpenAIHistoryTranscript(req["messages"].([]any))} {
				if !strings.Contains(text, "SOURCE_SENTINEL.txt") || !strings.Contains(text, "DISPLAY_SENTINEL") || strings.Contains(text, "QUJDRA==") {
					t.Fatalf("attachment context lost: %s", text)
				}
			}
		}
	}
}

func TestUnsupportedMediaFailsBeforeUpload(t *testing.T) {
	uploader := &nativeFileUploader{}
	req := map[string]any{"messages": []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_audio", "input_audio": map[string]any{"data": "QUJDRA=="}}}}}}
	err := PreprocessInlineFiles(context.Background(), &auth.RequestAuth{}, req, uploader, "deepseek-v4.1-flash", false)
	status, message := InlineFileErrorDetails(err)
	if err == nil || status != 400 || !strings.Contains(message, "input_audio") || len(uploader.uploads) != 0 {
		t.Fatalf("unsupported media proceeded: %v uploads=%d", err, len(uploader.uploads))
	}
}
