package client

import (
	"bytes"
	"context"
	"crypto/rand"
	dsprotocol "ds2api/internal/deepseek/protocol"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"path/filepath"
	"strconv"
	"strings"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	trans "ds2api/internal/deepseek/transport"
)

type UploadFileRequest struct {
	PhotoCompressRatio    *float64
	PictureCompressFormat string
	ThinkingEnabled       bool
	PreserveImage         bool
	Filename              string
	ContentType           string
	Purpose               string
	ModelType             string
	Data                  []byte
}

type UploadFileResult struct {
	ID         string
	Filename   string
	Bytes      int64
	Status     string
	Purpose    string
	AccountID  string
	IsImage    bool
	Raw        map[string]any
	RawHeaders http.Header
}

func (c *Client) UploadFile(ctx context.Context, a *auth.RequestAuth, req UploadFileRequest, maxAttempts int) (*UploadFileResult, error) {
	result, err := c.UploadFilePending(ctx, a, req, maxAttempts)
	if err != nil {
		return nil, err
	}
	if err := c.waitForUploadedFile(ctx, a, result); err != nil {
		return nil, err
	}
	return result, nil
}

// UploadFilePending lets shared inline preprocessing upload all files before
// waiting for them with a single repeated-file_ids request, as in the APK.
func (c *Client) UploadFilePending(ctx context.Context, a *auth.RequestAuth, req UploadFileRequest, maxAttempts int) (*UploadFileResult, error) {
	if len(req.Data) == 0 {
		return nil, errors.New("file is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	req = applyUploadSettings(req, a, c.uploadSettings(ctx, a))
	return c.reuseUploadedFile(ctx, a, req, maxAttempts)
}

func (c *Client) uploadFilePendingUncached(ctx context.Context, a *auth.RequestAuth, req UploadFileRequest, maxAttempts int) (*UploadFileResult, error) {
	if maxAttempts <= 0 {
		maxAttempts = c.maxRetries
	}
	var err error
	req, err = prepareUploadImage(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("prepare upload image: %w", err)
	}
	filename := req.Filename
	if strings.TrimSpace(filename) == "" {
		filename = "upload.bin"
	}
	contentType := strings.TrimSpace(req.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	purpose := strings.TrimSpace(req.Purpose)
	modelType := "default"
	bodyFactory, err := buildUploadMultipartBodyFactory(filename, contentType, req.Data)
	if err != nil {
		return nil, err
	}
	contentTypeHeader := bodyFactory.ContentType()
	capturePayload := map[string]any{
		"filename":     filename,
		"content_type": contentType,
		"purpose":      purpose,
		"bytes":        len(req.Data),
	}
	if modelType != "" {
		capturePayload["model_type"] = modelType
	}
	captureSession := c.capture.Start("deepseek_upload_file", dsprotocol.DeepSeekUploadFileURL, a.AccountID, capturePayload)
	attempts := 0
	powHeader := ""
	lastFailureKind := FailureUnknown
	lastFailureMessage := ""
	for attempts < maxAttempts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		clients := c.requestClientsForAuth(ctx, a)
		if strings.TrimSpace(powHeader) == "" {
			powHeader, err = c.GetPowForTarget(ctx, a, dsprotocol.DeepSeekUploadTargetPath, maxAttempts)
			if err != nil {
				return nil, err
			}
			clients = c.requestClientsForAuth(ctx, a)
		}
		headers := c.authHeadersForAuth(a)
		headers["Content-Type"] = contentTypeHeader
		if modelType != "" {
			headers["x-model-type"] = modelType
		}
		headers["x-ds-pow-response"] = powHeader
		headers["x-file-size"] = strconv.Itoa(len(req.Data))
		// The thinking flag belongs to both upload headers and completion JSON.
		if req.ThinkingEnabled {
			headers["x-thinking-enabled"] = "1"
		} else {
			headers["x-thinking-enabled"] = "0"
		}
		resp, err := c.doUpload(ctx, clients.regular, clients.fallback, dsprotocol.DeepSeekUploadFileURL, headers, bodyFactory.NewReader)
		if err != nil {
			config.Logger.Warn("[upload_file] request error", "error", err, "account", a.AccountID, "filename", filename)
			powHeader = ""
			lastFailureKind = FailureUnknown
			lastFailureMessage = err.Error()
			attempts++
			if !retryAfterNetworkError(ctx, attempts, err) {
				break
			}
			continue
		}
		if captureSession != nil {
			resp.Body = captureSession.WrapBody(resp.Body, resp.StatusCode)
		}
		payloadBytes, readErr := readResponseBody(resp)
		if closeErr := resp.Body.Close(); closeErr != nil {
			config.Logger.Warn("[upload_file] close response", "error", closeErr)
		}
		if readErr != nil {
			if errors.Is(readErr, ErrUpstreamResponseTooLarge) {
				return nil, readErr
			}
			powHeader = ""
			attempts++
			continue
		}
		parsed, err := decodeMetadataResponse(payloadBytes, resp.StatusCode)
		if err != nil {
			// An indeterminate upload response may already have stored the file.
			// Do not blindly repeat a potentially successful upload.
			return nil, err
		}
		code, bizCode, msg, bizMsg := extractResponseStatus(parsed)
		if resp.StatusCode == http.StatusOK && code == 0 && bizCode == 0 {
			result := extractUploadFileResult(parsed)
			result.Raw = parsed
			result.RawHeaders = resp.Header.Clone()
			if result.Filename == "" {
				result.Filename = filename
			}
			if result.Bytes == 0 {
				result.Bytes = int64(len(req.Data))
			}
			if result.Purpose == "" {
				result.Purpose = purpose
			}
			if result.AccountID == "" {
				result.AccountID = a.AccountID
			}
			if result.ID == "" {
				return nil, errors.New("upload file succeeded without file id")
			}
			return result, nil
		}
		config.Logger.Warn("[upload_file] failed", "status", resp.StatusCode, "code", code, "biz_code", bizCode, "msg", msg, "biz_msg", bizMsg, "account", a.AccountID, "filename", filename)
		powHeader = ""
		lastFailureMessage = failureMessage(msg, bizMsg, "upload file failed")
		if isTokenInvalid(resp.StatusCode, code, bizCode, msg, bizMsg) || isAuthIndicativeBizFailure(msg, bizMsg) {
			lastFailureKind = authFailureKind(a.UseConfigToken)
		} else {
			lastFailureKind = FailureUnknown
		}
		if kind := c.observeAccountFailure(a, parsed, "upload"); kind != FailureUnknown {
			if a.UseConfigToken && c.Auth != nil && c.Auth.SwitchAccount(ctx, a) {
				attempts++
				continue
			}
			return nil, &RequestFailure{Op: "upload", Kind: kind, Message: failureMessage(msg, bizMsg, "account unavailable")}
		}
		if c.recoverManagedAuth(ctx, a, &attempts, resp.StatusCode, code, bizCode, msg, bizMsg) {
			continue
		}
		attempts++
	}
	if lastFailureKind != FailureUnknown {
		return nil, &RequestFailure{Op: "upload file", Kind: lastFailureKind, Message: lastFailureMessage}
	}
	// Transport-level failures (context cancelled, connection reset) record
	// the underlying error text; surface it instead of a generic message.
	if lastFailureMessage != "" {
		return nil, errors.New(lastFailureMessage)
	}
	return nil, errors.New("upload file failed")
}

type uploadMultipartBodyFactory struct {
	prefix      []byte
	data        []byte
	suffix      []byte
	contentType string
}

func buildUploadMultipartBodyFactory(filename, contentType string, data []byte) (*uploadMultipartBodyFactory, error) {
	var prefix bytes.Buffer
	writer := multipart.NewWriter(&prefix)
	// 70 characters, fresh for each upload; captured random values are never reused.
	boundary := ""
	for len(boundary) < 70 {
		var entropy [4]byte
		if _, err := rand.Read(entropy[:]); err != nil {
			return nil, err
		}
		boundary += strconv.FormatInt(int64(int32(binary.BigEndian.Uint32(entropy[:]))), 16)
	}
	if err := writer.SetBoundary(boundary[:70]); err != nil {
		return nil, err
	}
	partHeader := textproto.MIMEHeader{}
	partHeader.Set("Content-Disposition", "form-data; name=file; filename="+appMultipartParameter(escapeMultipartFilename(filename)))
	partHeader.Set("Content-Type", "application/octet-stream")
	if _, err := writer.CreatePart(partHeader); err != nil {
		return nil, err
	}
	partPrefix := append([]byte(nil), prefix.Bytes()...)
	if err := writer.Close(); err != nil {
		return nil, err
	}
	full := prefix.Bytes()
	partSuffix := append([]byte(nil), full[len(partPrefix):]...)
	return &uploadMultipartBodyFactory{
		prefix:      partPrefix,
		data:        data,
		suffix:      partSuffix,
		contentType: writer.FormDataContentType(),
	}, nil
}

func (f *uploadMultipartBodyFactory) ContentType() string {
	return f.contentType
}

func (f *uploadMultipartBodyFactory) NewReader() io.Reader {
	return io.MultiReader(bytes.NewReader(f.prefix), bytes.NewReader(f.data), bytes.NewReader(f.suffix))
}

func buildUploadMultipartBody(filename, contentType string, data []byte) ([]byte, string, error) {
	factory, err := buildUploadMultipartBodyFactory(filename, contentType, data)
	if err != nil {
		return nil, "", err
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, factory.NewReader()); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), factory.ContentType(), nil
}

func escapeMultipartFilename(filename string) string {
	filename = filepath.Base(filename)
	if filename == "." || filename == "" {
		return "upload.bin"
	}
	return filename
}

// Ktor quote policy (APK a85.a/b), rather than Go's always-quoted %q.
func appMultipartParameter(value string) string {
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' && !strings.ContainsAny(value, "\r\n\t") {
		escaped, quoted := false, true
		for i := 1; i < len(value)-1; i++ {
			if value[i] == '"' && !escaped {
				quoted = false
				break
			}
			if value[i] == '\\' {
				escaped = !escaped
			} else {
				escaped = false
			}
		}
		if quoted {
			return value
		}
	}
	if value != "" && !strings.ContainsAny(value, "()<>@,;:\\\"/[]?={} \t\r\n") {
		return value
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\t", `\t`, "\r", `\r`, "\n", `\n`).Replace(value) + `"`
}

func (c *Client) doUpload(ctx context.Context, doer trans.Doer, fallback trans.Doer, url string, headers map[string]string, bodyFactory func() io.Reader) (*http.Response, error) {
	newReq := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bodyFactory())
		if err != nil {
			return nil, err
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		return req, nil
	}
	req, err := newReq()
	if err != nil {
		return nil, err
	}
	resp, err := doer.Do(req)
	if err == nil {
		return resp, nil
	}
	if trans.UseFramedH2() {
		return nil, err
	}
	config.Logger.Warn("[deepseek] fingerprint upload request failed, fallback to std transport", "url", url, "error", err)
	req2, reqErr := newReq()
	if reqErr != nil {
		return nil, reqErr
	}
	return fallback.Do(req2)
}

func extractUploadFileResult(resp map[string]any) *UploadFileResult {
	result := &UploadFileResult{Status: "uploaded"}
	data, _ := resp["data"].(map[string]any)
	bizData, _ := data["biz_data"].(map[string]any)
	searchMaps := []map[string]any{resp, data, bizData}
	for _, parent := range []map[string]any{resp, data, bizData} {
		if parent == nil {
			continue
		}
		for _, key := range []string{"file", "biz_data", "data"} {
			if nested, ok := parent[key].(map[string]any); ok {
				searchMaps = append(searchMaps, nested)
			}
		}
	}
	for _, m := range searchMaps {
		if m == nil {
			continue
		}
		if result.ID == "" {
			result.ID = firstNonEmptyString(m, "id", "file_id")
		}
		if result.Filename == "" {
			result.Filename = firstNonEmptyString(m, "name", "filename", "file_name")
		}
		if result.Status == "uploaded" {
			if status := firstNonEmptyString(m, "status", "file_status"); status != "" {
				result.Status = status
			}
		}
		if !result.IsImage {
			result.IsImage = firstBool(m, "is_image", "isImage")
		}
		if result.Purpose == "" {
			result.Purpose = firstNonEmptyString(m, "purpose")
		}
		if result.AccountID == "" {
			result.AccountID = firstNonEmptyString(m, "account_id", "accountId", "owner_account_id", "ownerAccountId")
		}
		if result.Bytes == 0 {
			result.Bytes = firstPositiveInt64(m, "bytes", "size", "file_size")
		}
	}
	return result
}

func firstBool(m map[string]any, keys ...string) bool {
	for _, key := range keys {
		switch v := m[key].(type) {
		case bool:
			return v
		case string:
			switch strings.ToLower(strings.TrimSpace(v)) {
			case "true", "1", "yes", "y":
				return true
			}
		}
	}
	return false
}

func firstNonEmptyString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, _ := m[key].(string); strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func firstPositiveInt64(m map[string]any, keys ...string) int64 {
	for _, key := range keys {
		if v := toInt64(m[key], 0); v > 0 {
			return v
		}
	}
	return 0
}
