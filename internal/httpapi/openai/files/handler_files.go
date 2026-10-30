package files

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"ds2api/internal/auth"
	"ds2api/internal/chathistory"
	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
	"ds2api/internal/httpapi/openai/shared"
	"ds2api/internal/util"
)

const openAIUploadMaxMemory = 32 << 20

type Handler struct {
	Store        shared.ConfigReader
	Auth         shared.AuthResolver
	DS           shared.DeepSeekCaller
	ChatHistory  *chathistory.Store
	ContentStore ContentStore
}

type fileFetcher interface {
	FetchUploadedFile(ctx context.Context, a *auth.RequestAuth, fileID string) (*dsclient.UploadFileResult, error)
}

func (h *Handler) UploadFile(w http.ResponseWriter, r *http.Request) {
	a, err := h.Auth.Determine(r)
	if err != nil {
		status := http.StatusUnauthorized
		detail := err.Error()
		if auth.IsNoIdleAccountError(err) {
			status = http.StatusTooManyRequests
		}
		shared.WriteOpenAIError(w, status, detail)
		return
	}
	defer h.Auth.Release(a)
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type"))), "multipart/form-data") {
		shared.WriteOpenAIError(w, http.StatusBadRequest, "content-type must be multipart/form-data")
		return
	}
	// Enforce a hard cap on the total request body size to prevent OOM
	r.Body = http.MaxBytesReader(w, r.Body, shared.UploadMaxSize)
	if err := r.ParseMultipartForm(openAIUploadMaxMemory); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "too large") {
			shared.WriteOpenAIError(w, http.StatusRequestEntityTooLarge, "file size exceeds limit")
			return
		}
		shared.WriteOpenAIError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	if r.MultipartForm != nil {
		defer func() {
			if err := r.MultipartForm.RemoveAll(); err != nil {
				config.Logger.Warn("[files] remove multipart temporary files", "error", err)
			}
		}()
	}
	r = r.WithContext(auth.WithAuth(r.Context(), a))
	file, header, err := r.FormFile("file")
	if err != nil {
		shared.WriteOpenAIError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer func() {
		if err := file.Close(); err != nil {
			config.Logger.Warn("[files] close uploaded file", "error", err)
		}
	}()
	data, err := io.ReadAll(file)
	if err != nil {
		shared.WriteOpenAIError(w, http.StatusBadRequest, "failed to read uploaded file")
		return
	}
	contentType := strings.TrimSpace(header.Header.Get("Content-Type"))
	if contentType == "" && len(data) > 0 {
		contentType = http.DetectContentType(data)
	}
	modelType := resolveUploadModelType(h.Store, r)
	pinFileAccount(a)
	result, err := h.DS.UploadFile(r.Context(), a, dsclient.UploadFileRequest{
		Filename:        header.Filename,
		ContentType:     contentType,
		Purpose:         strings.TrimSpace(r.FormValue("purpose")),
		ModelType:       modelType,
		ThinkingEnabled: resolveUploadThinking(h.Store, r),
		PreserveImage:   r.FormValue("preserve_image") == "true",
		Data:            data,
	}, 3)
	if err != nil {
		WriteInlineFileError(w, upstreamFileError(err, "Failed to upload file."))
		return
	}
	if result != nil && result.AccountID == "" {
		result.AccountID = a.AccountID
	}
	if h.ContentStore != nil && result != nil {
		if err := h.ContentStore.Store(result.ID, header.Filename, contentType, data); err != nil {
			// Upload succeeded upstream; losing the local cache is non-fatal.
			config.Logger.Warn("[files] failed to cache uploaded file content", "file_id", result.ID, "error", err)
		}
	}
	shared.WriteJSON(w, http.StatusOK, buildOpenAIFileObject(result))
}

func (h *Handler) RetrieveFile(w http.ResponseWriter, r *http.Request) {
	a, err := h.Auth.Determine(r)
	if err != nil {
		status := http.StatusUnauthorized
		detail := err.Error()
		if auth.IsNoIdleAccountError(err) {
			status = http.StatusTooManyRequests
		}
		shared.WriteOpenAIError(w, status, detail)
		return
	}
	defer h.Auth.Release(a)

	fileID := strings.TrimSpace(chi.URLParam(r, "file_id"))
	if fileID == "" {
		shared.WriteOpenAIError(w, http.StatusBadRequest, "file_id is required")
		return
	}
	fetcher, ok := h.DS.(fileFetcher)
	if !ok {
		shared.WriteOpenAIError(w, http.StatusNotImplemented, "file retrieval is not available")
		return
	}
	result, err := fetcher.FetchUploadedFile(r.Context(), a, fileID)
	if err != nil {
		if errors.Is(err, dsclient.ErrUploadFileNotFound) {
			shared.WriteOpenAIError(w, http.StatusNotFound, "file not found")
			return
		}
		WriteInlineFileError(w, upstreamFileError(err, "Failed to retrieve file."))
		return
	}
	if result != nil && result.AccountID == "" {
		result.AccountID = a.AccountID
	}
	shared.WriteJSON(w, http.StatusOK, buildOpenAIFileObject(result))
}

func resolveUploadModelType(store shared.ConfigReader, r *http.Request) string {
	for _, candidate := range []string{r.FormValue("model_type"), r.Header.Get("X-Model-Type")} {
		if modelType := normalizeUploadModelType(candidate); modelType != "" {
			return modelType
		}
	}
	requestedModel := strings.TrimSpace(r.FormValue("model"))
	if requestedModel != "" {
		if resolvedModel, ok := config.ResolveModel(store, requestedModel); ok {
			if modelType, ok := config.GetModelType(resolvedModel); ok {
				return modelType
			}
		}
	}
	return "default"
}

func normalizeUploadModelType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "default":
		return "default"
	case "expert", "vision":
		// The 2.4.0 web client merged expert/vision into the single upstream
		// "default" model_type; upload_file now always expects default.
		return "default"
	default:
		return ""
	}
}

func buildOpenAIFileObject(result *dsclient.UploadFileResult) map[string]any {
	if result == nil {
		obj := map[string]any{
			"id":             "",
			"object":         "file",
			"bytes":          0,
			"created_at":     time.Now().Unix(),
			"filename":       "",
			"purpose":        "",
			"status":         "uploaded",
			"status_details": nil,
		}
		return obj
	}
	obj := map[string]any{
		"id":             result.ID,
		"object":         "file",
		"bytes":          result.Bytes,
		"created_at":     time.Now().Unix(),
		"filename":       result.Filename,
		"purpose":        result.Purpose,
		"status":         result.Status,
		"status_details": nil,
	}
	if result.AccountID != "" {
		obj["account_id"] = result.AccountID
	}
	return obj
}

func resolveUploadThinking(store shared.ConfigReader, r *http.Request) bool {
	model := strings.TrimSpace(r.FormValue("model"))
	if model == "" {
		model = "deepseek-v4.1-flash"
	}
	resolved, ok := config.ResolveModel(store, model)
	if !ok {
		return false
	}
	defaultThinking, _, _ := config.GetModelConfig(resolved)
	request := map[string]any{}
	for _, value := range []string{r.FormValue("thinking_enabled"), r.Header.Get("X-Thinking-Enabled")} {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "1", "true":
			request["thinking"] = true
		case "0", "false":
			request["thinking"] = false
		default:
			continue
		}
		break
	}
	return util.ResolveThinkingEnabled(request, defaultThinking) && !config.IsNoThinkingModel(resolved)
}
