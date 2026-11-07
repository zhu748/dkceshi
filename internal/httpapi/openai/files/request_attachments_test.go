package files

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"ds2api/internal/auth"
	dsclient "ds2api/internal/deepseek/client"
	"ds2api/internal/promptcompat"
)

type requestAttachmentUploader struct{ uploads int }

func (u *requestAttachmentUploader) UploadFile(_ context.Context, _ *auth.RequestAuth, _ dsclient.UploadFileRequest, _ int) (*dsclient.UploadFileResult, error) {
	u.uploads++
	return &dsclient.UploadFileResult{ID: fmt.Sprintf("request-file-%d", u.uploads), Status: "SUCCESS"}, nil
}

func TestRequestLevelFilesUploadAndRetainTheirTextContext(t *testing.T) {
	for _, key := range promptcompat.AttachmentInputKeys() {
		req := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{map[string]any{"role": "user", "content": "TASK"}}, key: []any{
			map[string]any{"type": "input_file", "filename": "SOURCE_FILE.txt", "file_data": "data:text/plain;base64,SEVMTE8=", "description": "FILE_CONTEXT"},
		}}
		promptcompat.CaptureInputFieldDigests(req)
		uploader := &requestAttachmentUploader{}
		if err := PreprocessInlineFiles(context.Background(), &auth.RequestAuth{}, req, uploader, "deepseek-v4.1-flash", true); err != nil {
			t.Fatal(err)
		}
		standard, err := promptcompat.NormalizeOpenAIChatRequest(nil, req, "")
		if err != nil || uploader.uploads != 1 || len(standard.RefFileIDs) != 1 {
			t.Fatalf("%s not uploaded: uploads=%d refs=%v err=%v", key, uploader.uploads, standard.RefFileIDs, err)
		}
		for _, text := range []string{standard.FinalPrompt, promptcompat.BuildOpenAIHistoryTranscript(standard.Messages)} {
			if !strings.Contains(text, "SOURCE_FILE.txt") || !strings.Contains(text, "FILE_CONTEXT") || strings.Contains(text, "SEVMTE8=") {
				t.Fatalf("attachment text/binary handling failed: %s", text)
			}
		}
	}
}

func TestRequestLevelFilesRejectUnuploadedRemoteImages(t *testing.T) {
	req := map[string]any{"files": []any{map[string]any{"type": "input_image", "image_url": "https://example.com/image.png"}}}
	if err := PreprocessInlineFiles(context.Background(), &auth.RequestAuth{}, req, &requestAttachmentUploader{}, "deepseek-v4.1-flash", false); err == nil {
		t.Fatal("remote image silently disappeared")
	}
}

func TestRequestImageMetadataSurvivesUploadAndImageCleanup(t *testing.T) {
	for _, key := range promptcompat.AttachmentInputKeys() {
		req := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{map[string]any{"role": "user", "content": "TASK"}}, key: []any{
			map[string]any{"type": "input_image", "image_url": "data:image/png;base64,QUJDRA==", "filename": "PICTURE.png", "description": "IMAGE_CONTEXT"},
		}}
		route := promptcompat.PlanImageRoute(req, nil)
		if !route.Rerouted {
			t.Fatal("request-wide image not detected")
		}
		if err := PreprocessInlineFiles(context.Background(), &auth.RequestAuth{}, req, &requestAttachmentUploader{}, "deepseek-v4.1-flash", false); err != nil {
			t.Fatal(err)
		}
		route.StripImages(req)
		standard, err := promptcompat.NormalizeOpenAIChatRequest(nil, req, "")
		if err != nil || strings.Count(standard.FinalPrompt, "IMAGE_CONTEXT") != 1 || !strings.Contains(standard.FinalPrompt, "PICTURE.png") || len(standard.RefFileIDs) != 1 {
			t.Fatalf("image context changed after cleanup: %v %s", err, standard.FinalPrompt)
		}
	}
}
