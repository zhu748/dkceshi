package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	dsprotocol "ds2api/internal/deepseek/protocol"
)

const (
	fileReadyPollAttempts = 60
	fileReadyPollInterval = time.Second
	fileReadyPollTimeout  = 65 * time.Second
)

var fileReadySleep = func(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// ErrUploadFileNotFound indicates that DeepSeek returned no matching uploaded file.
var ErrUploadFileNotFound = errors.New("uploaded file not found")
var ErrUploadFileRejected = errors.New("uploaded file processing failed")

func (c *Client) waitForUploadedFile(ctx context.Context, a *auth.RequestAuth, result *UploadFileResult) error {
	return c.WaitForUploadedFiles(ctx, a, []*UploadFileResult{result})
}

// WaitForUploadedFiles polls pending files together with repeated file_ids
// parameters. Ready files leave the next poll, preserving caller order.
func (c *Client) WaitForUploadedFiles(ctx context.Context, a *auth.RequestAuth, results []*UploadFileResult) error {
	pending := make([]*UploadFileResult, 0, len(results))
	for _, result := range results {
		if result == nil || strings.TrimSpace(result.ID) == "" {
			continue
		}
		if a != nil && result.AccountID != "" && result.AccountID != a.AccountID {
			return errors.New("uploaded files belong to a different account")
		}
		if isFailedUploadFileStatus(result.Status) {
			return fmt.Errorf("%w: status=%s", ErrUploadFileRejected, result.Status)
		}
		if !isReadyUploadFileStatus(result.Status) {
			pending = append(pending, result)
		}
	}
	if len(pending) == 0 {
		return nil
	}
	pollCtx, cancel := context.WithTimeout(ctx, fileReadyPollTimeout)
	defer cancel()
	interval := c.uploadSettings(pollCtx, a).filePollInterval
	if interval <= 0 {
		interval = fileReadyPollInterval
	}

	var lastErr error
	for attempt := 0; attempt < fileReadyPollAttempts; attempt++ {
		if err := pollCtx.Err(); err != nil {
			return fmt.Errorf("waiting for file readiness: %w", err)
		}
		ids := make([]string, 0, len(pending))
		for _, result := range pending {
			ids = append(ids, result.ID)
		}
		// The readiness loop owns its retry budget; do not nest metadata retries.
		fetched, err := c.fetchUploadedFilesWithAuth(pollCtx, a, ids)
		if err == nil {
			remaining := pending[:0]
			for _, result := range pending {
				for _, file := range fetched {
					if file.ID == result.ID {
						mergeUploadFileResults(result, file)
						break
					}
				}
				if isFailedUploadFileStatus(result.Status) {
					return fmt.Errorf("%w: status=%s", ErrUploadFileRejected, result.Status)
				}
				if !isReadyUploadFileStatus(result.Status) {
					remaining = append(remaining, result)
				}
			}
			pending = remaining
			if len(pending) == 0 {
				return nil
			}
			lastErr = fmt.Errorf("%d files are still processing", len(pending))
		} else {
			if isPermanentFileFetchError(err) {
				return fmt.Errorf("waiting for file readiness: %w", err)
			}
			var limited *FileRateLimitError
			if errors.As(err, &limited) && a.ManagedPool() {
				return fmt.Errorf("waiting for file readiness: %w", err)
			}
			lastErr = err
			config.Logger.Debug("[upload_file] waiting for file readiness", "count", len(pending), "attempt", attempt+1, "error", err)
		}

		if attempt < fileReadyPollAttempts-1 {
			delay := interval
			var limited *FileRateLimitError
			if errors.As(lastErr, &limited) {
				delay = max(delay, fileRateLimitDelay(attempt+1, limited.RetryAfter))
			}
			if err := fileReadySleep(pollCtx, delay); err != nil {
				return fmt.Errorf("waiting for file readiness: %w", err)
			}
		}
	}

	return fmt.Errorf("files did not become ready: %w", lastErr)
}

// FetchUploadedFile returns metadata for an uploaded DeepSeek file by ID.
func (c *Client) FetchUploadedFile(ctx context.Context, a *auth.RequestAuth, fileID string) (*UploadFileResult, error) {
	files, err := c.FetchUploadedFiles(ctx, a, []string{fileID})
	if err != nil {
		return nil, err
	}
	return files[0], nil
}

func (c *Client) fetchUploadedFilesWithAuth(ctx context.Context, a *auth.RequestAuth, fileIDs []string) ([]*UploadFileResult, error) {
	files, err := c.fetchUploadedFiles(ctx, a, fileIDs)
	if IsManagedUnauthorizedError(err) && c.Auth != nil && c.Auth.RefreshTokenOnce(ctx, a) {
		// A file stays on its owning account even if that account refreshes auth.
		return c.fetchUploadedFiles(ctx, a, fileIDs)
	}
	return files, err
}

func (c *Client) fetchUploadedFiles(ctx context.Context, a *auth.RequestAuth, fileIDs []string) ([]*UploadFileResult, error) {
	params := url.Values{}
	ids := make([]string, 0, len(fileIDs))
	seen := make(map[string]bool, len(fileIDs))
	for _, id := range fileIDs {
		id = strings.TrimSpace(id)
		if id != "" && !seen[id] {
			params.Add("file_ids", id)
			ids = append(ids, id)
			seen[id] = true
		}
	}
	if len(ids) == 0 {
		return nil, errors.New("file id is required")
	}
	ctx = auth.WithAuth(ctx, a)
	clients := c.requestClientsForAuth(ctx, a)
	reqURL := dsprotocol.DeepSeekFetchFilesURL + "?" + params.Encode()
	headers := c.authHeadersForAuth(a)

	resp, status, responseHeaders, err := c.getJSONWithStatusHeaders(ctx, clients.regular, reqURL, headers)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound || status == http.StatusGone {
		return nil, ErrUploadFileNotFound
	}

	code, bizCode, msg, bizMsg := extractResponseStatus(resp)
	if status != http.StatusOK || code != 0 || bizCode != 0 {
		if limited := fileRateLimit("fetch files", status, code, bizCode, msg, bizMsg); limited != nil {
			limited.RetryAfter = responseHeaders.Get("Retry-After")
			a.NoteRateLimit()
			return nil, limited
		}
		if kind := c.observeAccountFailure(a, resp, "fetch files"); kind != FailureUnknown {
			return nil, &RequestFailure{Op: "fetch files", Kind: kind, Message: failureMessage(msg, bizMsg, "account unavailable")}
		}
		if isTokenInvalid(status, code, bizCode, msg, bizMsg) || isAuthIndicativeBizFailure(msg, bizMsg) {
			return nil, &RequestFailure{Op: "fetch files", Kind: authFailureKind(a != nil && a.UseConfigToken), Message: failureMessage(msg, bizMsg, "file authentication failed")}
		}
		if strings.TrimSpace(bizMsg) != "" {
			msg = bizMsg
		}
		if msg == "" {
			msg = http.StatusText(status)
		}
		return nil, &fileFetchError{status: status, message: fmt.Sprintf("request failed: status=%d, code=%d, msg=%s", status, code, msg)}
	}

	files := make([]*UploadFileResult, 0, len(ids))
	for _, id := range ids {
		result := extractFetchedUploadFileResult(resp, id)
		if result == nil || strings.TrimSpace(result.ID) == "" {
			continue
		}
		result.Raw = resp
		files = append(files, result)
	}
	if len(files) != len(ids) {
		if !hasFetchedFileList(resp) {
			return nil, fmt.Errorf("%w: missing files list", ErrInvalidUpstreamResponse)
		}
		return nil, fmt.Errorf("%w: requested %d files, received %d", ErrUploadFileNotFound, len(ids), len(files))
	}
	return files, nil
}

// Only a recognized file-list envelope proves absence. An empty or changed
// response schema is an error, not permission to upload the same bytes again.
func hasFetchedFileList(resp map[string]any) bool {
	if _, ok := resp["files"].([]any); ok {
		return true
	}
	for _, key := range []string{"data", "biz_data"} {
		if nested, ok := resp[key].(map[string]any); ok && hasFetchedFileList(nested) {
			return true
		}
	}
	return false
}

func extractFetchedUploadFileResult(resp map[string]any, targetID string) *UploadFileResult {
	targetID = strings.TrimSpace(targetID)
	if resp == nil || targetID == "" {
		return nil
	}

	var walk func(any) *UploadFileResult
	walk = func(v any) *UploadFileResult {
		switch x := v.(type) {
		case map[string]any:
			if result := buildUploadFileResultFromMap(x, targetID); result != nil {
				return result
			}
			for _, nested := range x {
				if result := walk(nested); result != nil {
					return result
				}
			}
		case []any:
			for _, item := range x {
				if result := walk(item); result != nil {
					return result
				}
			}
		}
		return nil
	}

	if result := walk(resp); result != nil {
		return result
	}
	return nil
}

func buildUploadFileResultFromMap(m map[string]any, targetID string) *UploadFileResult {
	fileID := strings.TrimSpace(firstNonEmptyString(m, "id", "file_id"))
	if fileID == "" || !strings.EqualFold(fileID, targetID) {
		return nil
	}
	result := &UploadFileResult{
		ID:       fileID,
		Filename: firstNonEmptyString(m, "name", "filename", "file_name"),
		Status:   firstNonEmptyString(m, "status", "file_status"),
		Purpose:  firstNonEmptyString(m, "purpose"),
		IsImage:  firstBool(m, "is_image", "isImage"),
		Bytes:    firstPositiveInt64(m, "bytes", "size", "file_size"),
	}
	if result.Status == "" {
		result.Status = "uploaded"
	}
	readFileTokenUsage(result, m)
	return result
}

func isReadyUploadFileStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "processed", "ready", "done", "available", "success", "completed", "finished":
		return true
	default:
		return false
	}
}

func isFailedUploadFileStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "failure", "error", "rejected", "deleted":
		return true
	default:
		return false
	}
}
