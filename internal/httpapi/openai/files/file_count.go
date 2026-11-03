package files

import (
	"context"

	"ds2api/internal/auth"
	dsclient "ds2api/internal/deepseek/client"
)

// CountFileInputs resolves existing IDs, but estimates inline payloads locally:
// asking for a token count must never upload or persist the user's file.
func CountFileInputs(ctx context.Context, a *auth.RequestAuth, req map[string]any, uploader FileUploader, model string, thinking bool) error {
	return PreprocessInlineFiles(ctx, a, req, countOnlyUploader{delegate: uploader}, model, thinking)
}

type countOnlyUploader struct {
	delegate FileUploader
}

func (u countOnlyUploader) UploadFile(_ context.Context, _ *auth.RequestAuth, req dsclient.UploadFileRequest, _ int) (*dsclient.UploadFileResult, error) {
	id := "count-file-" + inlineFileFingerprint(req.Filename, req.ContentType, req.Data)
	return &dsclient.UploadFileResult{ID: id, Bytes: int64(len(req.Data)), Status: "SUCCESS"}, nil
}

func (u countOnlyUploader) FetchUploadedFiles(ctx context.Context, a *auth.RequestAuth, ids []string) ([]*dsclient.UploadFileResult, error) {
	if fetcher, ok := u.delegate.(batchFileFetcher); ok {
		return fetcher.FetchUploadedFiles(ctx, a, ids)
	}
	files := make([]*dsclient.UploadFileResult, 0, len(ids))
	for _, id := range ids {
		file := &dsclient.UploadFileResult{ID: id}
		if fetcher, ok := u.delegate.(fileFetcher); ok {
			var err error
			file, err = fetcher.FetchUploadedFile(ctx, a, id)
			if err != nil {
				return nil, err
			}
		}
		files = append(files, file)
	}
	return files, nil
}
