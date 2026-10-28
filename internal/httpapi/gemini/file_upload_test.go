package gemini

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/auth"
	dsclient "ds2api/internal/deepseek/client"
)

type geminiPhotoDS struct {
	geminiMarkerDS
	uploads []dsclient.UploadFileRequest
}

func (d *geminiPhotoDS) UploadFile(_ context.Context, _ *auth.RequestAuth, req dsclient.UploadFileRequest, _ int) (*dsclient.UploadFileResult, error) {
	d.uploads = append(d.uploads, req)
	return &dsclient.UploadFileResult{ID: "photo-test"}, nil
}

func TestGeminiDirectUploadsNativePhotoBeforeCompletion(t *testing.T) {
	store, resolver := newGeminiMarkerResolver(t)
	ds := &geminiPhotoDS{geminiMarkerDS: geminiMarkerDS{toolCall: "ok"}}
	h := &Handler{Store: store, Auth: resolver, DS: ds}
	req := geminiMarkerRequest(t, `{"contents":[{"role":"user","parts":[{"text":"describe this"},{"inlineData":{"mimeType":"image/png","data":"QUJDRA=="}}]}]}`)
	rec := httptest.NewRecorder()
	h.GenerateContent(rec, req)
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if len(ds.uploads) != 1 || !ds.uploads[0].ThinkingEnabled || ds.uploads[0].ContentType != "image/png" {
		t.Fatalf("upload mismatch: %#v", ds.uploads)
	}
	ids, _ := ds.payload["ref_file_ids"].([]any)
	if len(ids) != 1 || ids[0] != "photo-test" {
		t.Fatalf("photo ID missing from completion: %#v", ids)
	}
	if prompt, _ := ds.payload["prompt"].(string); strings.Contains(prompt, "QUJDRA==") {
		t.Fatal("base64 leaked into prompt")
	}
}
