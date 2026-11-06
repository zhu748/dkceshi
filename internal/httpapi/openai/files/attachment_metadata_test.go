package files

import (
	"context"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
)

func TestUploadedDocumentKeepsContextInPromptAndHistory(t *testing.T) {
	req := map[string]any{"messages": []any{map[string]any{"role": "user", "content": []any{map[string]any{
		"type": "document", "title": "REPORT_TITLE", "context": "DOCUMENT_INSTRUCTION", "citations": map[string]any{"enabled": true},
		"signature": "OPAQUE_SIGNATURE", "source": map[string]any{"type": "base64", "media_type": "application/pdf", "data": "QUJDRA==", "description": "SOURCE_DESCRIPTION"},
	}}}}}
	uploader := &nativeFileUploader{}
	if err := PreprocessInlineFiles(context.Background(), &auth.RequestAuth{}, req, uploader, "deepseek-v4.1-flash", false); err != nil {
		t.Fatal(err)
	}
	messages := req["messages"].([]any)
	for _, text := range []string{promptcompat.RenderPromptText(messages, false), promptcompat.BuildOpenAIHistoryTranscript(messages)} {
		for _, want := range []string{"REPORT_TITLE", "DOCUMENT_INSTRUCTION", "SOURCE_DESCRIPTION", "citations", "file-test"} {
			if !strings.Contains(text, want) {
				t.Fatalf("attachment context %q missing: %s", want, text)
			}
		}
		if strings.Contains(text, "QUJDRA==") || strings.Contains(text, "OPAQUE_SIGNATURE") {
			t.Fatal("binary or opaque payload leaked into prompt")
		}
	}
	if len(uploader.uploads) != 1 || string(uploader.uploads[0].Data) != "ABCD" {
		t.Fatal("attachment bytes changed")
	}
}

func TestConvertedNativeMediaKeepsDisplayNameAfterUpload(t *testing.T) {
	original := map[string]any{"inlineData": map[string]any{"mimeType": "image/png", "data": "QUJDRA==", "displayName": "tool-image.png"}, "context": "IMAGE_DESCRIPTION"}
	converted, ok := promptcompat.NormalizeAttachmentContent(original)
	if !ok {
		t.Fatal("native media not recognized")
	}
	req := map[string]any{"messages": []any{map[string]any{"role": "tool", "content": []any{converted}}}}
	if err := PreprocessInlineFiles(context.Background(), &auth.RequestAuth{}, req, &nativeFileUploader{}, "deepseek-v4.1-flash", false); err != nil {
		t.Fatal(err)
	}
	history := promptcompat.BuildOpenAIHistoryTranscript(req["messages"].([]any))
	if !strings.Contains(history, "tool-image.png") || !strings.Contains(history, "IMAGE_DESCRIPTION") || strings.Contains(history, "QUJDRA==") {
		t.Fatalf("converted media context changed: %s", history)
	}
}
