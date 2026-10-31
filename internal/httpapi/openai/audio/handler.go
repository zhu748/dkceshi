package audio

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"ds2api/internal/assistantturn"
	"ds2api/internal/auth"
	"ds2api/internal/completionruntime"
	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
	"ds2api/internal/httpapi/openai/shared"
)

// DeepSeekCaller is the subset of the DeepSeek client this surface needs. It
// composes the shared completion runtime (for the recitation turn) with the
// TTS transport methods.
type DeepSeekCaller interface {
	completionruntime.DeepSeekCaller
	SynthesizeSpeech(ctx context.Context, a *auth.RequestAuth, sessionID string, messageID int, opts dsclient.TTSOptions) (*dsclient.TTSAudio, error)
	DeleteSessionForToken(ctx context.Context, token string, sessionID string) (*dsclient.DeleteSessionResult, error)
}

// ConfigReader is the read-only configuration this surface needs: model alias
// resolution for the recitation, the speech master switch, and the account pool
// in-flight ceiling.
type ConfigReader interface {
	config.ModelAliasReader
	config.TTSReader
	config.RuntimeAccountMaxInflightReader
}

// Handler serves POST /v1/audio/speech (OpenAI-compatible shape).
type Handler struct {
	Store ConfigReader
	Auth  shared.AuthResolver
	DS    DeepSeekCaller
}

func (h *Handler) Speech(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Auth == nil || h.DS == nil {
		shared.WriteOpenAIError(w, http.StatusInternalServerError, "tts handler is not configured")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxTTSRequestBody)
	var req speechRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "too large") {
			shared.WriteOpenAIError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		shared.WriteOpenAIError(w, http.StatusBadRequest, "invalid json")
		return
	}
	h.handleSpeech(w, r, req)
}

// SpeechGET is the GET form used by reader apps (e.g. Legado's speech engine),
// which only support a URL template: ?input=...&voice=...&response_format=...
func (h *Handler) SpeechGET(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Auth == nil || h.DS == nil {
		shared.WriteOpenAIError(w, http.StatusInternalServerError, "tts handler is not configured")
		return
	}
	q := r.URL.Query()
	input := firstNonEmpty(q.Get("input"), q.Get("text"), q.Get("speakText"))
	var speedPtr *float64
	if s := strings.TrimSpace(q.Get("speed")); s != "" {
		val, err := strconv.ParseFloat(s, 64)
		if err != nil {
			shared.WriteOpenAIError(w, http.StatusBadRequest, "invalid speed parameter")
			return
		}
		speedPtr = &val
	}
	// The default format (also for GET) is Ogg Opus; reader apps whose players
	// cannot handle Ogg pass response_format=wav explicitly.
	format := strings.TrimSpace(q.Get("response_format"))
	h.handleSpeech(w, r, speechRequest{
		Model:          q.Get("model"),
		Input:          input,
		Voice:          q.Get("voice"),
		ResponseFormat: format,
		Speed:          speedPtr,
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// audioQueryCredential maps the reader-app ?token= credential onto the
// Authorization header, but only when no header-based credential was sent, so
// the header keeps its precedence over query fallbacks.
func audioQueryCredential(r *http.Request) *http.Request {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" || hasHeaderCredential(r) {
		return r
	}
	rewritten := r.Clone(r.Context())
	rewritten.Header.Set("Authorization", "Bearer "+token)
	return rewritten
}

// hasHeaderCredential mirrors the header-based credentials the auth layer
// accepts (Authorization Bearer, x-api-key, x-goog-api-key).
func hasHeaderCredential(r *http.Request) bool {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(r.Header.Get("Authorization"))), "bearer ") {
		return true
	}
	return strings.TrimSpace(r.Header.Get("x-api-key")) != "" ||
		strings.TrimSpace(r.Header.Get("x-goog-api-key")) != ""
}

func (h *Handler) handleSpeech(w http.ResponseWriter, r *http.Request, req speechRequest) {
	r = audioQueryCredential(r)
	plan, outErr := h.prepareSpeech(req)
	if outErr != nil {
		writeOutputError(w, outErr)
		return
	}
	baseCtx := r.Context()
	runCtx, cancel := context.WithCancel(baseCtx)
	defer cancel()
	runReq := r.Clone(runCtx)
	// Prove the credential is valid without spending a pool rate-limit token.
	callerAuth, err := h.Auth.DetermineCaller(runReq)
	if err != nil {
		shared.WriteOpenAIError(w, http.StatusUnauthorized, err.Error())
		return
	}
	workers, seed, outErr := h.acquireSpeechWorkers(runReq, plan, callerAuth)
	if outErr != nil {
		writeOutputError(w, outErr)
		return
	}
	defer func() {
		if seed != nil {
			h.Auth.Release(seed)
		}
	}()
	for _, a := range workers {
		if a != seed {
			defer h.Auth.Release(a)
		}
	}
	outcomes, outErr := h.runSpeechWorkers(runCtx, cancel, workers, plan)
	if seed != nil {
		h.Auth.Release(seed)
		seed = nil
	}
	defer h.cleanupSpeechSessions(baseCtx, outcomes)
	if outErr != nil {
		writeOutputError(w, outErr)
		return
	}
	h.writeSpeechResponse(w, plan, outcomes)
}

func writeOutputError(w http.ResponseWriter, outErr *assistantturn.OutputError) {
	status := outErr.Status
	if status < 400 || status > 599 {
		status = http.StatusBadGateway
	}
	shared.WriteOpenAIErrorWithCode(w, status, outErr.Message, outErr.Code)
}
