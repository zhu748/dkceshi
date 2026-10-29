package files

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
	"ds2api/internal/httpapi/openai/shared"
	"ds2api/internal/promptcompat"
	"ds2api/internal/util"
)

const maxInlineFilesPerRequest = 50

type inlineFileUploadError struct {
	status  int
	message string
	err     error
}

func (e *inlineFileUploadError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.message) != "" {
		return e.message
	}
	if e.err != nil {
		return e.err.Error()
	}
	return "inline file processing failed"
}

type inlineUploadState struct {
	ctx             context.Context
	uploader        FileUploader
	auth            *auth.RequestAuth
	modelType       string
	thinkingEnabled bool
	preserveImage   bool
	uploadedByID    map[string]string
	uploadCount     int
	inlineFileBytes int
	pendingFiles    []*dsclient.UploadFileResult
}

type inlineDecodedFile struct {
	Data            []byte
	ContentType     string
	Filename        string
	ReplacementType string
}

func (h *Handler) PreprocessInlineFileInputs(ctx context.Context, a *auth.RequestAuth, req map[string]any) error {
	if h == nil || h.DS == nil || len(req) == 0 {
		return nil
	}
	model := "deepseek-v4.1-flash"
	if requested, ok := req["model"].(string); ok && strings.TrimSpace(requested) != "" {
		model = requested
	}
	resolved, ok := config.ResolveModel(h.Store, model)
	if !ok {
		resolved = model
	}
	defaultThinking, _, _ := config.GetModelConfig(resolved)
	thinking := util.ResolveThinkingEnabled(req, defaultThinking) && !config.IsNoThinkingModel(resolved)
	return PreprocessInlineFiles(ctx, a, req, h.DS, resolved, thinking)
}

// FileUploader is the shared upload boundary for every public protocol.
type FileUploader interface {
	UploadFile(context.Context, *auth.RequestAuth, dsclient.UploadFileRequest, int) (*dsclient.UploadFileResult, error)
}

type pendingFileUploader interface {
	UploadFilePending(context.Context, *auth.RequestAuth, dsclient.UploadFileRequest, int) (*dsclient.UploadFileResult, error)
	WaitForUploadedFiles(context.Context, *auth.RequestAuth, []*dsclient.UploadFileResult) error
}

func PreprocessInlineFiles(ctx context.Context, a *auth.RequestAuth, req map[string]any, uploader FileUploader, resolvedModel string, thinkingEnabled bool) error {
	if uploader == nil || len(req) == 0 {
		return nil
	}
	if len(promptcompat.CollectOpenAIRefFileIDs(req)) > 0 {
		pinFileAccount(a)
	}
	modelType, ok := config.GetModelType(resolvedModel)
	if !ok {
		modelType = "default"
	}
	state := &inlineUploadState{
		ctx:             ctx,
		uploader:        uploader,
		auth:            a,
		modelType:       modelType,
		thinkingEnabled: thinkingEnabled,
		preserveImage:   req["preserve_image"] == true,
		uploadedByID:    map[string]string{},
	}
	for _, key := range []string{"messages", "input", "attachments", "contents"} {
		if raw, ok := req[key]; ok {
			updated, err := state.walk(raw)
			if err != nil {
				return err
			}
			req[key] = updated
		}
	}
	if batch, ok := uploader.(pendingFileUploader); ok && len(state.pendingFiles) > 0 {
		if err := batch.WaitForUploadedFiles(ctx, a, state.pendingFiles); err != nil {
			return &inlineFileUploadError{status: http.StatusInternalServerError, message: "Failed to process inline files.", err: err}
		}
	}
	if refIDs := promptcompat.CollectOpenAIRefFileIDs(req); len(refIDs) > 0 {
		req["ref_file_ids"] = stringsToAnySlice(refIDs)
	}
	if state.inlineFileBytes > 0 {
		req["_inline_file_bytes"] = state.inlineFileBytes
	}
	return nil
}

func WriteInlineFileError(w http.ResponseWriter, err error) {
	status, message := InlineFileErrorDetails(err)
	shared.WriteOpenAIError(w, status, message)
}

func (s *inlineUploadState) walk(raw any) (any, error) {
	switch x := raw.(type) {
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			updated, err := s.walk(item)
			if err != nil {
				return nil, err
			}
			out[i] = updated
		}
		return out, nil
	case map[string]any:
		if replacement, replaced, err := s.tryUploadBlock(x); replaced || err != nil {
			return replacement, err
		}
		for _, key := range []string{"messages", "input", "attachments", "content", "files", "items", "data", "source", "file", "image_url", "contents", "parts"} {
			if nested, ok := x[key]; ok {
				updated, err := s.walk(nested)
				if err != nil {
					return nil, err
				}
				x[key] = updated
			}
		}
		return x, nil
	default:
		return raw, nil
	}
}

func (s *inlineUploadState) tryUploadBlock(block map[string]any) (map[string]any, bool, error) {
	decoded, ok, err := decodeOpenAIInlineFileBlock(block)
	if err != nil {
		return nil, true, &inlineFileUploadError{status: http.StatusBadRequest, message: err.Error(), err: err}
	}
	if !ok {
		return nil, false, nil
	}
	if s.uploadCount >= maxInlineFilesPerRequest {
		err := fmt.Errorf("exceeded maximum of %d inline files per request", maxInlineFilesPerRequest)
		return nil, true, &inlineFileUploadError{status: http.StatusBadRequest, message: err.Error(), err: err}
	}
	fileID, err := s.uploadInlineFile(decoded)
	if err != nil {
		status := http.StatusInternalServerError
		message := "Failed to upload inline file."
		if dsclient.IsMutedError(err) {
			status = http.StatusForbidden
			message = "Account is muted by upstream."
		} else if dsclient.IsManagedUnauthorizedError(err) {
			status = http.StatusUnauthorized
			message = "Account token is invalid. Please re-login the account in admin."
		} else if dsclient.IsDirectUnauthorizedError(err) {
			status = http.StatusUnauthorized
			message = "Invalid token. If this should be a DS2API key, add it to config.keys first."
		}
		return nil, true, &inlineFileUploadError{status: status, message: message, err: err}
	}
	s.uploadCount++
	s.inlineFileBytes += len(decoded.Data)
	replacement := map[string]any{
		"type":    decoded.ReplacementType,
		"file_id": fileID,
	}
	if decoded.Filename != "" {
		replacement["filename"] = decoded.Filename
	}
	if decoded.ContentType != "" {
		replacement["mime_type"] = decoded.ContentType
	}
	return replacement, true, nil
}

func (s *inlineUploadState) uploadInlineFile(file inlineDecodedFile) (string, error) {
	sum := sha256.Sum256(append([]byte(file.ContentType+"\x00"+file.Filename+"\x00"), file.Data...))
	cacheKey := fmt.Sprintf("%x", sum[:])
	if fileID, ok := s.uploadedByID[cacheKey]; ok && strings.TrimSpace(fileID) != "" {
		return fileID, nil
	}
	contentType := strings.TrimSpace(file.ContentType)
	if contentType == "" {
		contentType = http.DetectContentType(file.Data)
	}
	pinFileAccount(s.auth)
	request := dsclient.UploadFileRequest{
		Filename:        file.Filename,
		ContentType:     contentType,
		ModelType:       s.modelType,
		ThinkingEnabled: s.thinkingEnabled,
		PreserveImage:   s.preserveImage,
		Data:            file.Data,
	}
	upload := s.uploader.UploadFile
	if batch, ok := s.uploader.(pendingFileUploader); ok {
		upload = batch.UploadFilePending
	}
	result, err := upload(s.ctx, s.auth, request, 3)
	if err != nil {
		return "", err
	}
	if result == nil {
		return "", fmt.Errorf("upload succeeded without file metadata")
	}
	fileID := strings.TrimSpace(result.ID)
	if fileID == "" {
		return "", fmt.Errorf("upload succeeded without file id")
	}
	s.uploadedByID[cacheKey] = fileID
	s.pendingFiles = append(s.pendingFiles, result)
	return fileID, nil
}

func decodeOpenAIInlineFileBlock(block map[string]any) (inlineDecodedFile, bool, error) {
	if block == nil {
		return inlineDecodedFile{}, false, nil
	}
	if strings.TrimSpace(shared.AsString(block["file_id"])) != "" {
		return inlineDecodedFile{}, false, nil
	}
	if nested, ok := block["file"].(map[string]any); ok {
		decoded, matched, err := decodeOpenAIInlineFileBlock(nested)
		if err != nil || !matched {
			return decoded, matched, err
		}
		if decoded.Filename == "" {
			decoded.Filename = pickInlineFilename(block, decoded.ContentType, defaultInlinePrefix(decoded.ReplacementType))
		}
		return decoded, true, nil
	}
	blockType := strings.ToLower(strings.TrimSpace(shared.AsString(block["type"])))
	if decoded, matched, err := decodeNativeInlineBlock(block, blockType); matched || err != nil {
		return decoded, matched, err
	}
	if raw, matched := extractInlineImageDataURL(block); matched {
		data, contentType, err := decodeInlinePayload(raw, contentTypeFromMap(block))
		if err != nil {
			return inlineDecodedFile{}, true, fmt.Errorf("invalid image input")
		}
		return inlineDecodedFile{
			Data:            data,
			ContentType:     contentType,
			Filename:        pickInlineFilename(block, contentType, "image"),
			ReplacementType: "input_image",
		}, true, nil
	}
	if raw, matched := extractInlineFilePayload(block, blockType); matched {
		data, contentType, err := decodeInlinePayload(raw, contentTypeFromMap(block))
		if err != nil {
			return inlineDecodedFile{}, true, fmt.Errorf("invalid file input")
		}
		return inlineDecodedFile{
			Data:            data,
			ContentType:     contentType,
			Filename:        pickInlineFilename(block, contentType, defaultInlinePrefix(blockType)),
			ReplacementType: "input_file",
		}, true, nil
	}
	return inlineDecodedFile{}, false, nil
}

func extractInlineImageDataURL(block map[string]any) (string, bool) {
	imageURL := block["image_url"]
	switch x := imageURL.(type) {
	case string:
		if isDataURL(x) {
			return strings.TrimSpace(x), true
		}
	case map[string]any:
		if raw := strings.TrimSpace(shared.AsString(x["url"])); isDataURL(raw) {
			return raw, true
		}
	}
	if raw := strings.TrimSpace(shared.AsString(block["url"])); isDataURL(raw) {
		return raw, true
	}
	return "", false
}

func extractInlineFilePayload(block map[string]any, blockType string) (string, bool) {
	for _, value := range []any{block["file_data"], block["base64"], block["data"]} {
		if raw := strings.TrimSpace(shared.AsString(value)); raw != "" {
			if strings.Contains(blockType, "file") || block["file_data"] != nil || block["filename"] != nil || block["file_name"] != nil || block["name"] != nil {
				return raw, true
			}
		}
	}
	return "", false
}

func decodeInlinePayload(raw string, explicitContentType string) ([]byte, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, "", fmt.Errorf("empty payload")
	}
	if isDataURL(raw) {
		return decodeDataURL(raw, explicitContentType)
	}
	decoded, err := decodeBase64Flexible(raw)
	if err != nil {
		return nil, "", err
	}
	contentType := strings.TrimSpace(explicitContentType)
	if contentType == "" && len(decoded) > 0 {
		contentType = http.DetectContentType(decoded)
	}
	return decoded, contentType, nil
}

func decodeDataURL(raw string, explicitContentType string) ([]byte, string, error) {
	raw = strings.TrimSpace(raw)
	if !isDataURL(raw) {
		return nil, "", fmt.Errorf("unsupported data url")
	}
	header, payload, ok := strings.Cut(raw, ",")
	if !ok {
		return nil, "", fmt.Errorf("invalid data url")
	}
	meta := strings.TrimSpace(strings.TrimPrefix(header, "data:"))
	contentType := strings.TrimSpace(explicitContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
		if meta != "" {
			parts := strings.Split(meta, ";")
			if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
				contentType = strings.TrimSpace(parts[0])
			}
		}
	}
	if strings.Contains(strings.ToLower(meta), ";base64") {
		decoded, err := decodeBase64Flexible(payload)
		if err != nil {
			return nil, "", err
		}
		return decoded, contentType, nil
	}
	decoded, err := url.PathUnescape(payload)
	if err != nil {
		return nil, "", err
	}
	return []byte(decoded), contentType, nil
}

func decodeBase64Flexible(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		decoded, err := enc.DecodeString(raw)
		if err == nil {
			return decoded, nil
		}
	}
	return nil, fmt.Errorf("invalid base64 payload")
}

func contentTypeFromMap(block map[string]any) string {
	for _, value := range []any{block["mime_type"], block["mimeType"], block["content_type"], block["contentType"], block["media_type"], block["mediaType"]} {
		if contentType := strings.TrimSpace(shared.AsString(value)); contentType != "" {
			return contentType
		}
	}
	if imageURL, ok := block["image_url"].(map[string]any); ok {
		for _, value := range []any{imageURL["mime_type"], imageURL["mimeType"], imageURL["content_type"], imageURL["contentType"]} {
			if contentType := strings.TrimSpace(shared.AsString(value)); contentType != "" {
				return contentType
			}
		}
	}
	return ""
}

func pickInlineFilename(block map[string]any, contentType string, prefix string) string {
	for _, value := range []any{block["filename"], block["file_name"], block["name"]} {
		if name := strings.TrimSpace(shared.AsString(value)); name != "" {
			return filepath.Base(name)
		}
	}
	if prefix == "" {
		prefix = "upload"
	}
	ext := ".bin"
	if parsedType := strings.TrimSpace(contentType); parsedType != "" {
		if comma := strings.Index(parsedType, ";"); comma >= 0 {
			parsedType = strings.TrimSpace(parsedType[:comma])
		}
		if exts, err := mime.ExtensionsByType(parsedType); err == nil && len(exts) > 0 && strings.TrimSpace(exts[0]) != "" {
			ext = exts[0]
		}
	}
	return prefix + ext
}

func defaultInlinePrefix(blockType string) string {
	blockType = strings.ToLower(strings.TrimSpace(blockType))
	if strings.Contains(blockType, "image") {
		return "image"
	}
	return "upload"
}

func isDataURL(raw string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(raw)), "data:")
}

func stringsToAnySlice(items []string) []any {
	out := make([]any, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Uploaded IDs are installation/account scoped. Reuse the resolver's existing
// explicit-target constraint so a later PoW/retry cannot mix files from two
// accounts in one completion. Generated HISTORY/TOOLS use their rebuild path.
func pinFileAccount(a *auth.RequestAuth) {
	if a != nil && a.UseConfigToken && strings.TrimSpace(a.TargetAccount) == "" {
		a.TargetAccount = a.AccountID
	}
}
