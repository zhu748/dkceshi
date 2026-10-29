package files

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"ds2api/internal/auth"
	dsclient "ds2api/internal/deepseek/client"
)

type batchInlineUploader struct {
	uploads int
	waits   int
	fail    bool
}

func (u *batchInlineUploader) UploadFile(context.Context, *auth.RequestAuth, dsclient.UploadFileRequest, int) (*dsclient.UploadFileResult, error) {
	return nil, errors.New("shared preprocessing should defer readiness")
}

func (u *batchInlineUploader) UploadFilePending(_ context.Context, a *auth.RequestAuth, r dsclient.UploadFileRequest, _ int) (*dsclient.UploadFileResult, error) {
	if a.TargetAccount != a.AccountID || !r.ThinkingEnabled {
		return nil, errors.New("lost account or thinking flag")
	}
	u.uploads++
	return &dsclient.UploadFileResult{ID: fmt.Sprintf("file-%d", u.uploads), Status: "PENDING"}, nil
}

func (u *batchInlineUploader) WaitForUploadedFiles(_ context.Context, _ *auth.RequestAuth, files []*dsclient.UploadFileResult) error {
	u.waits++
	if u.uploads != 2 || len(files) != 2 {
		return errors.New("polled before all files were uploaded")
	}
	if u.fail {
		return errors.New("file processing rejected")
	}
	return nil
}

func TestSharedInlineFilesWaitAsOneBatchAndPropagateProcessingFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		uploader := &batchInlineUploader{fail: fail}
		req := map[string]any{"messages": []any{map[string]any{"role": "user", "content": []any{
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,QUJDRA=="}},
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,RUZHSA=="}},
		}}}}
		a := &auth.RequestAuth{AccountID: "account", UseConfigToken: true}
		err := PreprocessInlineFiles(context.Background(), a, req, uploader, "deepseek-v4.1-flash", true)
		if (err != nil) != fail || uploader.waits != 1 {
			t.Fatalf("batch fail=%v err=%v waits=%d", fail, err, uploader.waits)
		}
	}
}
