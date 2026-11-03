package files

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"

	"ds2api/internal/auth"
	dsclient "ds2api/internal/deepseek/client"
	"ds2api/internal/promptcompat"
)

type usageFileUploader struct {
	uploads, fetches, waits int
	fetchedIDs              []string
	fetchErr                error
	account                 string
	known                   bool
	ready                   bool
}

func (u *usageFileUploader) UploadFile(ctx context.Context, a *auth.RequestAuth, req dsclient.UploadFileRequest, attempts int) (*dsclient.UploadFileResult, error) {
	return u.UploadFilePending(ctx, a, req, attempts)
}

func (u *usageFileUploader) UploadFilePending(context.Context, *auth.RequestAuth, dsclient.UploadFileRequest, int) (*dsclient.UploadFileResult, error) {
	u.uploads++
	return &dsclient.UploadFileResult{ID: "file-image", Status: "PENDING", Bytes: 67652}, nil
}

func (u *usageFileUploader) FetchUploadedFiles(_ context.Context, a *auth.RequestAuth, ids []string) ([]*dsclient.UploadFileResult, error) {
	u.fetches++
	u.fetchedIDs = append([]string(nil), ids...)
	if !a.AccountPinned() {
		return nil, errors.New("file owner not pinned")
	}
	if u.fetchErr != nil {
		return nil, u.fetchErr
	}
	var files []*dsclient.UploadFileResult
	for _, id := range ids {
		file := &dsclient.UploadFileResult{ID: id, Status: "PENDING", AccountID: u.account, Bytes: 300}
		if u.ready {
			tokens := 954
			file.Status, file.TokenUsage = "SUCCESS", &tokens
		}
		files = append(files, file)
	}
	return files, nil
}

func (u *usageFileUploader) WaitForUploadedFiles(_ context.Context, _ *auth.RequestAuth, files []*dsclient.UploadFileResult) error {
	u.waits++
	for _, file := range files {
		file.Status = "SUCCESS"
		if u.known {
			tokens := 954
			if file.ID == "file-zero" {
				tokens = 0
			}
			file.TokenUsage = &tokens
		}
	}
	return nil
}

func TestFileUsageUsesReadyMetadataAndDeduplicatesAcrossInputs(t *testing.T) {
	u := &usageFileUploader{known: true}
	image := func() any {
		return map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/webp;base64,QUJDRA=="}}
	}
	req := map[string]any{"file_ids": []any{"file-image", "file-zero", "file-zero"}, "messages": []any{map[string]any{"role": "user", "content": []any{image(), image()}}}, "_inline_file_bytes": 999999}
	a := &auth.RequestAuth{AccountID: "owner", UseConfigToken: true}
	if err := PreprocessInlineFiles(context.Background(), a, req, u, "deepseek-v4.1-flash", true); err != nil {
		t.Fatal(err)
	}
	if got := promptcompat.EstimateInlineFileTokens(req); got != 954 {
		t.Fatalf("got %d tokens", got)
	}
	if u.uploads != 1 || u.fetches != 1 || u.waits != 1 || !reflect.DeepEqual(u.fetchedIDs, []string{"file-zero"}) {
		t.Fatalf("unexpected upload/fetch/wait counts: %+v", u)
	}
	if _, ok := req["_inline_file_bytes"]; ok {
		t.Fatal("untrusted byte estimate retained")
	}
}

func TestFileReferencesReuseWithoutUploadsAndKeepFallback(t *testing.T) {
	for _, known := range []bool{true, false} {
		u := &usageFileUploader{known: known}
		req := map[string]any{"file_ids": []any{"file-existing", "file-existing"}}
		a := &auth.RequestAuth{AccountID: "owner", UseConfigToken: true}
		if err := PreprocessInlineFiles(context.Background(), a, req, u, "deepseek-v4.1-flash", false); err != nil {
			t.Fatal(err)
		}
		want := 100
		if known {
			want = 954
		}
		if got := promptcompat.EstimateInlineFileTokens(req); got != want || u.uploads != 0 || u.fetches != 1 {
			t.Fatalf("known=%v tokens=%d uploader=%+v", known, got, u)
		}
	}
}

func TestFileReferencesPreserveErrorsAndAccountIsolation(t *testing.T) {
	for _, tc := range []struct {
		err     error
		account string
		status  int
	}{
		{dsclient.ErrUploadFileNotFound, "", http.StatusNotFound},
		{dsclient.ErrUploadFileRejected, "", http.StatusUnprocessableEntity},
		{&dsclient.RequestFailure{Kind: dsclient.FailureManagedUnauthorized}, "", http.StatusUnauthorized},
		{nil, "another-account", http.StatusBadRequest},
	} {
		u := &usageFileUploader{fetchErr: tc.err, account: tc.account}
		req := map[string]any{"file_ids": []any{"file-existing"}}
		err := PreprocessInlineFiles(context.Background(), &auth.RequestAuth{AccountID: "owner", UseConfigToken: true}, req, u, "deepseek-v4.1-flash", false)
		status, _ := InlineFileErrorDetails(err)
		if err == nil || status != tc.status || u.uploads != 0 {
			t.Fatalf("case=%+v status=%d err=%v", tc, status, err)
		}
	}
}

func TestFileObjectIncludesNullableTokenUsage(t *testing.T) {
	zero := 0
	for _, tokens := range []*int{nil, &zero} {
		obj := buildOpenAIFileObject(&dsclient.UploadFileResult{ID: "file-test", TokenUsage: tokens, IsImage: true})
		if obj["token_usage"] != tokens || obj["is_image"] != true {
			t.Fatalf("metadata missing: %+v", obj)
		}
	}
}

func TestCountFileInputsNeverUploadsInlineData(t *testing.T) {
	u := &usageFileUploader{ready: true}
	req := map[string]any{"file_ids": []any{"file-existing"}, "messages": []any{map[string]any{"role": "user", "content": []any{
		map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": "QUJDRA=="}},
	}}}}
	err := CountFileInputs(context.Background(), &auth.RequestAuth{AccountID: "owner", UseConfigToken: true}, req, u, "deepseek-v4.1-flash", false)
	if err != nil {
		t.Fatal(err)
	}
	if tokens := promptcompat.EstimateInlineFileTokens(req); tokens != 955 || u.uploads != 0 || u.fetches != 1 || u.waits != 0 {
		t.Fatalf("count-only operation tokens=%d uploader=%+v", tokens, u)
	}
}
