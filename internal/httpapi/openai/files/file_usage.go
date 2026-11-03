package files

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"ds2api/internal/auth"
	dsclient "ds2api/internal/deepseek/client"
	"ds2api/internal/promptcompat"
)

type batchFileFetcher interface {
	FetchUploadedFiles(context.Context, *auth.RequestAuth, []string) ([]*dsclient.UploadFileResult, error)
}

// Uploaded and explicitly referenced files share readiness, account binding and
// usage accounting. Each ID contributes once, regardless of its message count.
func (s *inlineUploadState) finalizeFileInputs(req map[string]any) error {
	delete(req, "_inline_file_bytes")
	ids := promptcompat.CollectOpenAIRefFileIDs(req)
	if err := checkFileReferenceLimit(ids); err != nil {
		return err
	}
	files := make(map[string]*dsclient.UploadFileResult, len(ids))
	for _, file := range s.pendingFiles {
		files[file.ID] = file
	}
	missing := make([]string, 0)
	for _, id := range ids {
		if files[id] == nil {
			missing = append(missing, id)
		}
	}
	if err := s.fetchReferencedFiles(missing, files); err != nil {
		return upstreamFileError(err, "Failed to retrieve referenced files.")
	}
	all := make([]*dsclient.UploadFileResult, 0, len(files))
	for _, id := range ids {
		if file := files[id]; file != nil {
			if rejectedFileStatus(file.Status) {
				return upstreamFileError(dsclient.ErrUploadFileRejected, "Referenced file processing failed.")
			}
			if s.auth != nil && file.AccountID != "" && file.AccountID != s.auth.AccountID {
				return &inlineFileUploadError{status: http.StatusBadRequest, message: "referenced files belong to a different account"}
			}
			all = append(all, file)
		}
	}
	if batch, ok := s.uploader.(pendingFileUploader); ok && len(all) > 0 {
		if err := batch.WaitForUploadedFiles(s.ctx, s.auth, all); err != nil {
			return upstreamFileError(err, "Failed to process referenced files.")
		}
	}
	counts := make(map[string]int, len(all))
	for _, file := range all {
		if file.TokenUsage != nil && *file.TokenUsage >= 0 {
			counts[file.ID] = *file.TokenUsage
		} else if bytes := s.fallbackBytes[file.ID]; bytes > 0 {
			counts[file.ID] = bytes / 3
		} else if file.Bytes > 0 {
			counts[file.ID] = int(min(file.Bytes/3, int64(^uint(0)>>1)))
		}
	}
	promptcompat.SetReferenceFileTokens(req, counts)
	if len(ids) > 0 {
		req["ref_file_ids"] = stringsToAnySlice(ids)
	}
	return nil
}

func (s *inlineUploadState) fetchReferencedFiles(ids []string, files map[string]*dsclient.UploadFileResult) error {
	if len(ids) == 0 {
		return nil
	}
	var fetched []*dsclient.UploadFileResult
	if batch, ok := s.uploader.(batchFileFetcher); ok {
		results, err := batch.FetchUploadedFiles(s.ctx, s.auth, ids)
		if err != nil {
			return err
		}
		fetched = results
	} else if fetcher, ok := s.uploader.(fileFetcher); ok {
		for _, id := range ids {
			file, err := fetcher.FetchUploadedFile(s.ctx, s.auth, id)
			if err != nil {
				return err
			}
			fetched = append(fetched, file)
		}
	} else {
		return nil // Alternate upload adapters may not support metadata lookup.
	}
	for _, file := range fetched {
		if file != nil {
			files[file.ID] = file
		}
	}
	for _, id := range ids {
		if files[id] == nil {
			return dsclient.ErrUploadFileNotFound
		}
	}
	return nil
}

func referenceFileErrorStatus(err error) int {
	if errors.Is(err, context.DeadlineExceeded) {
		return http.StatusGatewayTimeout
	}
	if errors.Is(err, dsclient.ErrInvalidUpstreamResponse) || errors.Is(err, dsclient.ErrUpstreamResponseTooLarge) {
		return http.StatusBadGateway
	}
	if errors.Is(err, dsclient.ErrUploadFileNotFound) {
		return http.StatusNotFound
	}
	if errors.Is(err, dsclient.ErrUploadFileRejected) {
		return http.StatusUnprocessableEntity
	}
	var statusError interface{ HTTPStatus() int }
	if errors.As(err, &statusError) {
		if status := statusError.HTTPStatus(); status >= 400 && status <= 599 {
			return status
		}
	}
	return http.StatusInternalServerError
}

func checkFileReferenceLimit(ids []string) error {
	if len(ids) > maxInlineFilesPerRequest {
		return &inlineFileUploadError{status: http.StatusBadRequest, message: fmt.Sprintf("exceeded maximum of %d referenced files per request", maxInlineFilesPerRequest)}
	}
	return nil
}

func rejectedFileStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "failure", "error", "rejected", "deleted":
		return true
	default:
		return false
	}
}
