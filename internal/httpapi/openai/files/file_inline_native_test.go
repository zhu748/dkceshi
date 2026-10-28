package files

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"ds2api/internal/auth"
	dsclient "ds2api/internal/deepseek/client"
	"ds2api/internal/promptcompat"
)

type nativeFileUploader struct{ uploads []dsclient.UploadFileRequest }

func (u *nativeFileUploader) UploadFile(_ context.Context, _ *auth.RequestAuth, req dsclient.UploadFileRequest, _ int) (*dsclient.UploadFileResult, error) {
	u.uploads = append(u.uploads, req)
	return &dsclient.UploadFileResult{ID: "file-test"}, nil
}

func TestNativeInlineFilesUseSharedUploadAndReferences(t *testing.T) {
	for _, body := range []string{
		`{"messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"QUJDRA=="}}]}]}`,
		`{"messages":[{"role":"user","content":[{"type":"document","source":{"type":"base64","media_type":"application/pdf","data":"QUJDRA=="}}]}]}`,
		`{"contents":[{"role":"user","parts":[{"inlineData":{"mimeType":"image/png","data":"QUJDRA=="}}]}]}`,
	} {
		for _, thinking := range []bool{true, false} {
			var req map[string]any
			if err := json.Unmarshal([]byte(body), &req); err != nil {
				t.Fatal(err)
			}
			uploader := &nativeFileUploader{}
			a := &auth.RequestAuth{UseConfigToken: true, AccountID: "test-account"}
			if err := PreprocessInlineFiles(context.Background(), a, req, uploader, "deepseek-v4.1-flash", thinking); err != nil {
				t.Fatal(err)
			}
			if len(uploader.uploads) != 1 || string(uploader.uploads[0].Data) != "ABCD" || uploader.uploads[0].ThinkingEnabled != thinking || uploader.uploads[0].ModelType != "default" {
				t.Fatalf("upload mismatch: %#v", uploader.uploads)
			}
			if a.TargetAccount != "test-account" {
				t.Fatal("file account was not pinned")
			}
			refs := promptcompat.CollectOpenAIRefFileIDs(req)
			if len(refs) != 1 || refs[0] != "file-test" {
				t.Fatalf("file references missing: %v", refs)
			}
			encoded, err := json.Marshal(req)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), "QUJDRA==") {
				t.Fatal("base64 left in prompt-visible request")
			}
		}
	}
}
