package files

import (
	"context"

	"ds2api/internal/auth"
	dsclient "ds2api/internal/deepseek/client"
	"ds2api/internal/promptcompat"
)

// Keep the original inline bytes and request shape until preprocessing succeeds.
// A failed partial batch is rebuilt entirely on the replacement account.
func PreprocessInlineFiles(ctx context.Context, a *auth.RequestAuth, req map[string]any, uploader FileUploader, model string, thinking bool) error {
	if uploader == nil || len(req) == 0 {
		return nil
	}
	originalTarget := ""
	if a != nil {
		originalTarget = a.TargetAccount
	}
	canMove := originalTarget == "" && len(promptcompat.CollectOpenAIRefFileIDs(req)) == 0
	for {
		working := cloneFileInput(req).(map[string]any)
		err := preprocessInlineFilesOnce(ctx, a, working, uploader, model, thinking)
		if err == nil {
			for k := range req {
				delete(req, k)
			}
			for k, v := range working {
				req[k] = v
			}
			return nil
		}
		if !canMove || !a.ManagedPool() {
			return err
		}
		a.TargetAccount = originalTarget
		if !dsclient.RetryAccountFailure(ctx, a, err) {
			return err
		}
	}
}

func cloneFileInput(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, item := range v {
			out[k] = cloneFileInput(item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = cloneFileInput(item)
		}
		return out
	default:
		return value
	}
}

func uploadStandaloneFile(ctx context.Context, a *auth.RequestAuth, uploader FileUploader, req dsclient.UploadFileRequest) (*dsclient.UploadFileResult, error) {
	originalTarget := a.TargetAccount
	for {
		pinFileAccount(a)
		file, err := uploader.UploadFile(ctx, a, req, 3)
		if err == nil || originalTarget != "" || !a.ManagedPool() {
			return file, err
		}
		a.TargetAccount = originalTarget
		if !dsclient.RetryAccountFailure(ctx, a, err) {
			return nil, err
		}
	}
}
