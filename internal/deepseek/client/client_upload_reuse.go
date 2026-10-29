package client

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"ds2api/internal/auth"
)

func (c *Client) reuseUploadedFile(ctx context.Context, a *auth.RequestAuth, req UploadFileRequest, maxAttempts int) (*UploadFileResult, error) {
	if c.uploadCache == nil || a == nil || a.DeepSeekToken == "" {
		return c.uploadFilePendingUncached(ctx, a, req, maxAttempts)
	}
	key := uploadReuseKey(a, req)
	return c.uploadCache.do(ctx, key, uploadReuseOwner(a.DeepSeekToken), func(cached *UploadFileResult) (*UploadFileResult, bool, error) {
		if cached != nil {
			current, err := c.FetchUploadedFile(ctx, a, cached.ID)
			if IsManagedUnauthorizedError(err) && c.Auth != nil && c.Auth.RefreshToken(ctx, a) {
				// Refresh only the owning account; file IDs never move accounts.
				current, err = c.FetchUploadedFile(ctx, a, cached.ID)
			}
			if err == nil && current != nil && !isFailedUploadFileStatus(current.Status) {
				mergeUploadFileResults(cached, current)
				return cached, key == uploadReuseKey(a, req), nil
			}
			if err != nil && !errors.Is(err, ErrUploadFileNotFound) {
				// An auth/transport failure is not proof that the file expired.
				return nil, false, err
			}
			c.uploadCache.forget(key)
		}
		file, err := c.uploadFilePendingUncached(ctx, a, req, maxAttempts)
		// Upload retries may refresh credentials or change accounts. Never lend
		// that result to another request still holding the original identity.
		return file, key == uploadReuseKey(a, req), err
	})
}

func uploadReuseOwner(token string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(token)))
}

func uploadReuseKey(a *auth.RequestAuth, req UploadFileRequest) string {
	headers := BaseHeadersForAuth(a)
	ratio := os.Getenv("DS2API_DEEPSEEK_PHOTO_COMPRESS_RATIO")
	if req.PhotoCompressRatio != nil {
		ratio = strconv.FormatFloat(*req.PhotoCompressRatio, 'g', -1, 64)
	}
	format := req.PictureCompressFormat
	if format == "" {
		format = os.Getenv("DS2API_DEEPSEEK_PICTURE_COMPRESS_FORMAT")
	}
	// Hash original bytes before lossy conversion and random filename creation.
	// Include names and MIME because they can change upstream document parsing.
	fields := []string{
		a.CallerID, a.AccountID, uploadReuseOwner(a.DeepSeekToken),
		headers["x-device-id"], headers["x-rangers-id"],
		req.Filename, strings.ToLower(strings.TrimSpace(req.ContentType)), req.Purpose,
		strconv.FormatBool(req.ThinkingEnabled), strconv.FormatBool(req.PreserveImage), ratio, format,
		fmt.Sprintf("%x", sha256.Sum256(req.Data)),
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%q", fields))))
}
