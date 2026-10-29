package client

import (
	"context"
	dsprotocol "ds2api/internal/deepseek/protocol"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ds2api/internal/auth"
	"ds2api/internal/config"
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
		fetched, err := c.FetchUploadedFiles(pollCtx, a, ids)
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
			lastErr = err
			config.Logger.Debug("[upload_file] waiting for file readiness", "count", len(pending), "attempt", attempt+1, "error", err)
		}

		if attempt < fileReadyPollAttempts-1 {
			if err := fileReadySleep(pollCtx, interval); err != nil {
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

func (c *Client) FetchUploadedFiles(ctx context.Context, a *auth.RequestAuth, fileIDs []string) ([]*UploadFileResult, error) {
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

	resp, status, err := c.getJSONWithStatus(ctx, clients.regular, reqURL, headers)
	if err != nil {
		return nil, err
	}

	code, bizCode, msg, bizMsg := extractResponseStatus(resp)
	if status != http.StatusOK || code != 0 || bizCode != 0 {
		if strings.TrimSpace(bizMsg) != "" {
			msg = bizMsg
		}
		if msg == "" {
			msg = http.StatusText(status)
		}
		return nil, fmt.Errorf("request failed: status=%d, code=%d, msg=%s", status, code, msg)
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
	if len(files) == 0 {
		return nil, ErrUploadFileNotFound
	}
	return files, nil
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
	return result
}

func mergeUploadFileResults(dst, src *UploadFileResult) {
	if dst == nil || src == nil {
		return
	}
	if strings.TrimSpace(src.ID) != "" {
		dst.ID = strings.TrimSpace(src.ID)
	}
	if strings.TrimSpace(src.Filename) != "" {
		dst.Filename = strings.TrimSpace(src.Filename)
	}
	if src.Bytes > 0 {
		dst.Bytes = src.Bytes
	}
	if strings.TrimSpace(src.Status) != "" {
		dst.Status = strings.TrimSpace(src.Status)
	}
	if strings.TrimSpace(src.Purpose) != "" {
		dst.Purpose = strings.TrimSpace(src.Purpose)
	}
	dst.IsImage = src.IsImage
	if len(src.Raw) > 0 {
		dst.Raw = src.Raw
	}
	if src.RawHeaders != nil {
		dst.RawHeaders = src.RawHeaders.Clone()
	}
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
