package files

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"ds2api/internal/auth"
)

type upstreamStatusError int

func (e upstreamStatusError) Error() string   { return fmt.Sprintf("upstream status %d", e) }
func (e upstreamStatusError) HTTPStatus() int { return int(e) }

func TestFileErrorsPreserveUpstreamRetryStatus(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		err := fmt.Errorf("file validation: %w", upstreamStatusError(status))
		got, _ := InlineFileErrorDetails(upstreamFileError(err, "failed"))
		if got != status {
			t.Fatalf("upstream %d became HTTP %d", status, got)
		}
	}
}

func TestTooManyReferencesFailBeforeAnyUpload(t *testing.T) {
	u := &usageFileUploader{}
	ids := make([]any, 51)
	for i := range ids {
		ids[i] = fmt.Sprintf("file-%d", i)
	}
	req := map[string]any{"file_ids": ids, "messages": []any{map[string]any{"role": "user", "content": []any{
		map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,QUJDRA=="}},
	}}}}
	err := PreprocessInlineFiles(context.Background(), &auth.RequestAuth{}, req, u, "deepseek-v4.1-flash", false)
	status, _ := InlineFileErrorDetails(err)
	if err == nil || status != 400 || u.uploads != 0 || u.fetches != 0 {
		t.Fatalf("invalid request caused upstream work: status=%d uploader=%+v err=%v", status, u, err)
	}
}

func TestFileErrorsKeepTimeoutSemantics(t *testing.T) {
	err := fmt.Errorf("wait for processing: %w", context.DeadlineExceeded)
	status, _ := InlineFileErrorDetails(upstreamFileError(err, "failed"))
	if status != http.StatusGatewayTimeout || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout became %d", status)
	}
}
