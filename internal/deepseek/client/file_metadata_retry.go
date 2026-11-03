package client

import (
	"context"
	"errors"

	"ds2api/internal/auth"
)

// FetchUploadedFiles also serves cache validation and explicit file references,
// which have no outer readiness loop. Retry rate limits here without treating a
// rejected metadata request as evidence that the file must be uploaded again.
func (c *Client) FetchUploadedFiles(ctx context.Context, a *auth.RequestAuth, fileIDs []string) ([]*UploadFileResult, error) {
	queryCtx, cancel := context.WithTimeout(ctx, fileReadyPollTimeout)
	defer cancel()
	const maxAttempts = 3
	for attempt := 1; ; attempt++ {
		if err := queryCtx.Err(); err != nil {
			return nil, err
		}
		files, err := c.fetchUploadedFilesWithAuth(queryCtx, a, fileIDs)
		var limited *FileRateLimitError
		if !errors.As(err, &limited) || attempt >= maxAttempts || a.ManagedPool() {
			return files, err
		}
		if err := fileReadySleep(queryCtx, fileRateLimitDelay(attempt, limited.RetryAfter)); err != nil {
			return nil, err
		}
	}
}
