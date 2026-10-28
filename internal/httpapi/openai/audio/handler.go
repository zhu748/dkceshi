package audio

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"ds2api/internal/account"
	"ds2api/internal/assistantturn"
	"ds2api/internal/auth"
	"ds2api/internal/completionruntime"
	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
	"ds2api/internal/httpapi/openai/shared"
	"ds2api/internal/promptcompat"
)

const (
	// OpenAI's /v1/audio/speech caps input at 4096 characters. Keep the same
	// bound: every recitation is a full completion, so oversized input also
	// means a large token and audio bill.
	maxTTSInputRunes  = 4096
	maxTTSRequestBody = 1 << 20
	// maxTTSSegmentRunes keeps each upstream message below the TTS service's
	// roughly one-minute audio limit. 160 Chinese characters are about 40 s of
	// speech, leaving headroom for a slower voice or a longer recitation.
	maxTTSSegmentRunes       = 160
	minTTSSegmentRunes       = 30
	maxTTSSegmentRunesConfig = 500
	maxTTSSegments           = 32
	// defaultTTSModel is the cheapest web model with thinking disabled. The
	// recitation only needs verbatim copying, not reasoning.
	defaultTTSModel  = "deepseek-v4.1-flash-nothinking"
	ttsReciteSurface = "tts"
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

type speechRequest struct {
	Model          string   `json:"model"`
	Input          string   `json:"input"`
	Voice          string   `json:"voice"`
	ResponseFormat string   `json:"response_format"`
	Speed          *float64 `json:"speed"`
}

// voiceAliases maps the display names users see in the DeepSeek web client
// (and the stock OpenAI voice names) onto the four upstream ids. The canonical
// ids are always accepted as-is.
var voiceAliases = map[string]string{
	"贝壳": dsclient.TTSVoiceMira,
	"白浪": dsclient.TTSVoiceEcho,
	"海星": dsclient.TTSVoiceStella,
	"暗潮": dsclient.TTSVoiceTide,

	"alloy":   dsclient.TTSVoiceMira,
	"echo":    dsclient.TTSVoiceEcho,
	"fable":   dsclient.TTSVoiceStella,
	"onyx":    dsclient.TTSVoiceTide,
	"nova":    dsclient.TTSVoiceStella,
	"shimmer": dsclient.TTSVoiceMira,
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
	// Reader-app URL templates pass the credential as ?token=, a parameter name
	// the shared auth layer deliberately does not accept. Map it onto the
	// Authorization header here so this surface keeps supporting it without
	// relaxing every other endpoint.
	r = audioQueryCredential(r)
	if strings.TrimSpace(req.Input) == "" {
		shared.WriteOpenAIError(w, http.StatusBadRequest, "input is required")
		return
	}
	if utf8.RuneCountInString(req.Input) > maxTTSInputRunes {
		shared.WriteOpenAIError(w, http.StatusBadRequest, fmt.Sprintf("input must be at most %d characters", maxTTSInputRunes))
		return
	}
	if h.Store != nil && !h.Store.TTSEnabled() {
		shared.WriteOpenAIErrorWithCode(w, http.StatusServiceUnavailable, "speech synthesis is disabled", "tts_disabled")
		return
	}
	voice, err := normalizeVoice(req.Voice)
	if err != nil {
		shared.WriteOpenAIError(w, http.StatusBadRequest, err.Error())
		return
	}
	format, err := normalizeFormat(req.ResponseFormat)
	if err != nil {
		shared.WriteOpenAIError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Speed != nil && *req.Speed != 1 {
		shared.WriteOpenAIError(w, http.StatusBadRequest, "speed is not supported by the upstream voice")
		return
	}
	model, err := h.resolveModel(req.Model)
	if err != nil {
		shared.WriteOpenAIError(w, http.StatusBadRequest, err.Error())
		return
	}

	segmentChars := maxTTSSegmentRunes
	multiAccount := true
	accountConcurrency := true
	if h.Store != nil {
		if n := h.Store.TTSSegmentChars(); n > 0 {
			segmentChars = n
		}
		multiAccount = h.Store.TTSMultiAccountEnabled()
		accountConcurrency = h.Store.TTSAccountConcurrencyEnabled()
	}
	if segmentChars < minTTSSegmentRunes {
		segmentChars = minTTSSegmentRunes
	} else if segmentChars > maxTTSSegmentRunesConfig {
		segmentChars = maxTTSSegmentRunesConfig
	}
	perAccount := resolvePerAccountLimit(h.Store, accountConcurrency)

	baseCtx := r.Context()
	runCtx, cancel := context.WithCancel(baseCtx)
	defer cancel()

	// Validate the credential without touching the account pool: DetermineCaller
	// checks it but takes no slot, so merely proving the key is valid no longer
	// spends a per-minute / 24h rate-limit token. A request with no credential at
	// all still fails with 401 here.
	seedReq := r.Clone(runCtx)
	callerAuth, callerErr := h.Auth.DetermineCaller(seedReq)
	if callerErr != nil {
		shared.WriteOpenAIError(w, http.StatusUnauthorized, callerErr.Error())
		return
	}

	// Single-account mode needs one concrete account up front so every worker
	// pins to it. Keep that slot instead of releasing and re-acquiring it: the
	// gap let another request take the account, after which the workers failed
	// with "no account" and the whole request returned 502. The seed is handed
	// to the first worker and released once the worker pool is done.
	var seed *auth.RequestAuth
	fixedAccountID := ""
	if !multiAccount {
		seed, err = h.Auth.Determine(seedReq)
		if err != nil {
			status := http.StatusUnauthorized
			if auth.IsNoIdleAccountError(err) {
				status = http.StatusTooManyRequests
			}
			shared.WriteOpenAIError(w, status, err.Error())
			return
		}
		fixedAccountID = seed.AccountID
		// Pin the seed too: the recitation runs before SynthesizeSpeech, and an
		// unpinned auth could switch accounts mid-request, which would spread a
		// single-account request over several accounts.
		seed.TargetAccount = fixedAccountID
		defer func() {
			if seed != nil {
				h.Auth.Release(seed)
			}
		}()
	}

	// The upstream TTS stream fails with service_error when one message is too
	// long (roughly a minute of audio). Split the input on sentence boundaries
	// and synthesize each segment separately, then join the Opus packets into
	// one Ogg stream.
	segments := splitTTSText(req.Input, segmentChars)
	if len(segments) == 0 {
		shared.WriteOpenAIError(w, http.StatusBadRequest, "input is required")
		return
	}
	if len(segments) > maxTTSSegments {
		shared.WriteOpenAIError(w, http.StatusBadRequest, fmt.Sprintf("input produces too many segments (%d > %d), please reduce input length or increase segment_chars in settings", len(segments), maxTTSSegments))
		return
	}

	workerLimit := len(segments)
	if multiAccount {
		accounts := 1
		filter := account.AccountFilter(func(acc config.Account) bool {
			return acc.MatchesPoolType(false)
		})
		if limiter, ok := h.Auth.(interface {
			SchedulableAccountsMatching(account.AccountFilter) int
		}); ok {
			if n := limiter.SchedulableAccountsMatching(filter); n > 0 {
				accounts = n
			}
		} else if limiter, ok := h.Auth.(interface{ SchedulableAccounts() int }); ok {
			if n := limiter.SchedulableAccounts(); n > 0 {
				accounts = n
			}
		}
		if n := accounts * perAccount; n > 0 && n < workerLimit {
			workerLimit = n
		}
	} else if perAccount < workerLimit {
		workerLimit = perAccount
	}
	if limiter, ok := h.Auth.(interface{ GlobalMaxInflight() int }); ok {
		if g := limiter.GlobalMaxInflight(); g > 0 && g < workerLimit {
			workerLimit = g
		}
	}
	// Direct-token callers bypass the pool, so only the per-account limit
	// applies to them.
	if callerAuth != nil && strings.TrimSpace(callerAuth.DeepSeekToken) != "" {
		if workerLimit > perAccount {
			workerLimit = perAccount
		}
	}
	if workerLimit < 1 {
		workerLimit = 1
	}

	gate := newAccountGate(perAccount)

	type segmentOutcome struct {
		frames    [][]byte
		text      string
		voiceID   string
		audioID   string
		sessionID string
		auth      *auth.RequestAuth
	}
	outcomes := make([]segmentOutcome, len(segments))
	var wg sync.WaitGroup
	var failOnce sync.Once
	var failedOutErr *assistantturn.OutputError
	var failedErr error
	fail := func(err error, outErr *assistantturn.OutputError) {
		failOnce.Do(func() {
			failedErr = err
			failedOutErr = outErr
			cancel()
		})
	}

	process := func(index int, a *auth.RequestAuth) {
		release, err := gate.acquire(runCtx, a.AccountID)
		if err != nil {
			fail(err, nil)
			return
		}
		defer release()
		ctx := auth.WithAuth(runCtx, a)
		recited, outErr := h.recite(ctx, a, model, segments[index])
		outcomes[index].auth = a
		outcomes[index].sessionID = recited.SessionID
		if outErr != nil {
			fail(nil, outErr)
			return
		}
		audio, err := h.DS.SynthesizeSpeech(ctx, a, recited.SessionID, recited.MessageID, dsclient.TTSOptions{
			Voice:  voice,
			Format: "opus",
		})
		if err != nil {
			config.Logger.Warn("[tts] synthesize failed",
				"account", a.AccountID, "session_id", recited.SessionID, "message_id", recited.MessageID,
				"segment", index+1, "segments", len(segments), "error", err)
			fail(err, nil)
			return
		}
		outcomes[index].frames = audio.Frames
		outcomes[index].text = recited.Text
		outcomes[index].voiceID = audio.VoiceID
		outcomes[index].audioID = audio.AudioID
	}

	// Each worker owns one account for the whole request and pulls segments off
	// the queue, so the pool sees one acquire per worker instead of one per
	// segment. Per-minute and 24h account rate limits count every acquire, and
	// acquiring per segment made a single long request burn its own budget and
	// then fail mid-flight once the window was exhausted. RequestAuth's mutable
	// bookkeeping is unsynchronized, which stays safe because a worker's auth
	// is only ever touched by its own goroutine.
	var workersAuth []*auth.RequestAuth
	if seed != nil {
		workersAuth = append(workersAuth, seed)
	}

	// Ensure at least one worker has a valid account (blocking wait if necessary).
	if len(workersAuth) == 0 {
		workerReq := r.Clone(runCtx)
		if fixedAccountID != "" {
			workerReq.Header.Set("X-Ds2-Target-Account", fixedAccountID)
		}
		a, err := h.Auth.Determine(workerReq)
		if err != nil {
			status := http.StatusBadGateway
			if auth.IsNoIdleAccountError(err) {
				status = http.StatusTooManyRequests
			}
			shared.WriteOpenAIError(w, status, err.Error())
			return
		}
		workersAuth = append(workersAuth, a)
	}

	// Try to acquire more accounts non-blockingly up to workerLimit for concurrency.
	// If the account pool is at capacity, stop acquiring and process the segments in batches
	// across the ready workers instead of deadlocking or failing.
	for len(workersAuth) < workerLimit {
		workerReq := r.Clone(runCtx)
		if fixedAccountID != "" {
			workerReq.Header.Set("X-Ds2-Target-Account", fixedAccountID)
		}
		var a *auth.RequestAuth
		var err error
		if tryResolver, ok := h.Auth.(interface {
			DetermineTry(*http.Request) (*auth.RequestAuth, error)
		}); ok {
			a, err = tryResolver.DetermineTry(workerReq)
		} else {
			a, err = h.Auth.Determine(workerReq)
		}
		if err != nil {
			break
		}
		workersAuth = append(workersAuth, a)
	}

	for _, a := range workersAuth {
		if a != seed {
			defer h.Auth.Release(a)
		}
	}

	segmentQueue := make(chan int, len(segments))
	for i := range segments {
		segmentQueue <- i
	}
	close(segmentQueue)
	for _, workerAuth := range workersAuth {
		wg.Add(1)
		go func(a *auth.RequestAuth) {
			defer wg.Done()
			for {
				select {
				case <-runCtx.Done():
					return
				case index, ok := <-segmentQueue:
					if !ok {
						return
					}
					process(index, a)
				}
			}
		}(workerAuth)
	}
	wg.Wait()
	if seed != nil {
		// Release the single-account seed after the worker that used it has
		// stopped. The deferred release sees nil and does nothing.
		h.Auth.Release(seed)
		seed = nil
	}

	// Recitation sessions exist only to make the message readable. Delete them
	// asynchronously after the audio is drained so cleanup cannot delay the
	// client response or breach serverless execution limits.
	defer func() {
		var sessionsToDelete []struct {
			auth      *auth.RequestAuth
			sessionID string
		}
		for _, outcome := range outcomes {
			if outcome.sessionID != "" && outcome.auth != nil {
				sessionsToDelete = append(sessionsToDelete, struct {
					auth      *auth.RequestAuth
					sessionID string
				}{auth: outcome.auth, sessionID: outcome.sessionID})
			}
		}
		if len(sessionsToDelete) == 0 {
			return
		}
		go func() {
			cleanCtx, cancelClean := context.WithTimeout(context.WithoutCancel(baseCtx), 15*time.Second)
			defer cancelClean()
			var cleanWg sync.WaitGroup
			cleanSem := make(chan struct{}, 8)
			for _, item := range sessionsToDelete {
				cleanWg.Add(1)
				go func(a *auth.RequestAuth, sID string) {
					defer cleanWg.Done()
					select {
					case cleanSem <- struct{}{}:
						defer func() { <-cleanSem }()
					case <-cleanCtx.Done():
						return
					}
					h.deleteSession(auth.WithAuth(cleanCtx, a), a, sID)
				}(item.auth, item.sessionID)
			}
			cleanWg.Wait()
		}()
	}()

	if failedOutErr != nil {
		writeOutputError(w, failedOutErr)
		return
	}
	if failedErr != nil {
		status := http.StatusBadGateway
		if auth.IsNoIdleAccountError(failedErr) {
			status = http.StatusTooManyRequests
		}
		shared.WriteOpenAIError(w, status, failedErr.Error())
		return
	}

	var frames [][]byte
	var recitedParts []string
	lastVoiceID := ""
	lastAudioID := ""
	for i, outcome := range outcomes {
		config.Logger.Debug("[tts] segment frames",
			"segment", i+1, "segments", len(outcomes), "frames", len(outcome.frames))
		frames = append(frames, outcome.frames...)
		if outcome.text != "" {
			recitedParts = append(recitedParts, outcome.text)
		}
		if outcome.voiceID != "" {
			lastVoiceID = outcome.voiceID
		}
		if outcome.audioID != "" {
			lastAudioID = outcome.audioID
		}
	}
	config.Logger.Info("[tts] synthesized",
		"segments", len(segments), "frames", len(frames), "voice", lastVoiceID)
	var audioBytes []byte
	contentType := "audio/ogg"
	if format == "wav" {
		audioBytes, err = dsclient.EncodeWAVFromOpus(frames)
		contentType = "audio/wav"
	} else {
		audioBytes, err = dsclient.EncodeOggOpus(frames)
	}
	if err != nil {
		config.Logger.Warn("[tts] encode audio failed", "format", format, "error", err)
		shared.WriteOpenAIErrorWithCode(w, http.StatusBadGateway, "failed to encode audio", "tts_encode_error")
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(audioBytes)))
	w.Header().Set("Cache-Control", "no-store")
	// Expose the TTS metadata to cross-origin callers as well; the admin page
	// is same-origin, but the endpoint is usable from any OpenAI-compatible
	// client that happens to run in a browser.
	w.Header().Set("Access-Control-Expose-Headers", "X-Ds2-Tts-Voice, X-Ds2-Tts-Audio-Id, X-Ds2-Tts-Format, X-Ds2-Tts-Text-Base64, X-Ds2-Tts-Segments, X-Ds2-Tts-Frames")
	if lastVoiceID != "" {
		w.Header().Set("X-Ds2-Tts-Voice", lastVoiceID)
	}
	if lastAudioID != "" {
		w.Header().Set("X-Ds2-Tts-Audio-Id", lastAudioID)
	}
	w.Header().Set("X-Ds2-Tts-Format", format)
	w.Header().Set("X-Ds2-Tts-Segments", strconv.Itoa(len(segments)))
	w.Header().Set("X-Ds2-Tts-Frames", strconv.Itoa(len(frames)))
	// The model may rewrite the recitation; expose what was actually spoken
	// so callers can verify it. Base64 keeps arbitrary text out of the header
	// grammar. Headers are capped at a few KB by most proxies, so skip the
	// field when it would be oversized.
	recitedText := strings.Join(recitedParts, "\n")
	if encoded := base64.RawURLEncoding.EncodeToString([]byte(recitedText)); encoded != "" && len(encoded) <= 4096 {
		w.Header().Set("X-Ds2-Tts-Text-Base64", encoded)
	}
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(audioBytes); err != nil {
		config.Logger.Warn("[tts] write response failed", "error", err)
	}
}

type reciteResult struct {
	SessionID string
	MessageID int
	Text      string
}

// recite asks the web model to copy the input verbatim, then returns the
// assistant message id that the TTS WebSocket can read.
func (h *Handler) recite(ctx context.Context, a *auth.RequestAuth, model, input string) (reciteResult, *assistantturn.OutputError) {
	prompt := buildRecitePrompt(input)
	stdReq := promptcompat.StandardRequest{
		Surface:         ttsReciteSurface,
		RequestedModel:  model,
		ResolvedModel:   model,
		ResponseModel:   model,
		Messages:        []any{map[string]any{"role": "user", "content": prompt}},
		FinalPrompt:     prompt,
		PromptTokenText: prompt,
		Thinking:        false,
		Search:          false,
	}
	result, outErr := completionruntime.ExecuteNonStreamWithRetry(ctx, h.DS, a, stdReq, completionruntime.Options{
		MaxAttempts:  3,
		RetryEnabled: true,
	})
	if outErr != nil {
		return reciteResult{SessionID: result.SessionID}, outErr
	}
	if strings.TrimSpace(result.Turn.Text) == "" {
		return reciteResult{SessionID: result.SessionID}, &assistantturn.OutputError{
			Status:  http.StatusBadGateway,
			Message: "upstream recitation produced empty text",
			Code:    "tts_empty_recitation",
		}
	}
	// TTS reads the assistant message the completion started with. When the
	// reply was long enough to trigger auto-continue, ResponseMessageID names
	// the last continuation segment instead, and the TTS endpoint answers that
	// one with service_error.
	messageID := result.Turn.FirstResponseMessageID
	if messageID <= 0 {
		messageID = result.Turn.ResponseMessageID
	}
	if messageID <= 0 {
		return reciteResult{SessionID: result.SessionID}, &assistantturn.OutputError{
			Status:  http.StatusBadGateway,
			Message: "upstream did not return a response message id",
			Code:    "tts_upstream_error",
		}
	}
	return reciteResult{
		SessionID: result.SessionID,
		MessageID: messageID,
		Text:      result.Turn.Text,
	}, nil
}

func buildRecitePrompt(input string) string {
	return "请把下面的内容原样复述一遍，用于语音朗读。要求：\n" +
		"1. 只输出复述内容，不要解释、不要加引号、不要用代码块。\n" +
		"2. 保留原文的文字和顺序，不要翻译、不要总结、不要改写。\n" +
		"3. 如果原文包含 Markdown 标记（例如 **、#、`），去掉标记，只保留可朗读的文字。\n\n" +
		input
}

func (h *Handler) resolveModel(raw string) (string, error) {
	model := strings.TrimSpace(raw)
	if model == "" || strings.HasPrefix(strings.ToLower(model), "tts-") {
		model = defaultTTSModel
	}
	resolved, ok := config.ResolveModel(h.Store, model)
	if !ok {
		return "", fmt.Errorf("model %q is not available", raw)
	}
	return resolved, nil
}

func (h *Handler) deleteSession(ctx context.Context, a *auth.RequestAuth, sessionID string) {
	if sessionID == "" || a == nil || strings.TrimSpace(a.DeepSeekToken) == "" {
		return
	}
	deleteCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if _, err := h.DS.DeleteSessionForToken(deleteCtx, a.DeepSeekToken, sessionID); err != nil {
		config.Logger.Warn("[tts] delete recitation session failed",
			"account", a.AccountID, "session_id", sessionID, "error", err)
	}
}

func normalizeVoice(raw string) (string, error) {
	voice := strings.ToLower(strings.TrimSpace(raw))
	if voice == "" {
		return dsclient.TTSVoiceMira, nil
	}
	if dsclient.IsValidTTSVoice(voice) {
		return voice, nil
	}
	if mapped, ok := voiceAliases[voice]; ok {
		return mapped, nil
	}
	return "", fmt.Errorf("unsupported voice %q (supported: %s; Chinese: 贝壳/白浪/海星/暗潮)", raw, strings.Join(dsclient.TTSVoices(), ", "))
}

func normalizeFormat(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "opus", "ogg", "ogg_opus":
		return "opus", nil
	case "wav", "wave":
		return "wav", nil
	default:
		return "", fmt.Errorf("unsupported response_format %q (supported: opus, wav)", raw)
	}
}

// resolvePerAccountLimit caps concurrent segment generations per account for TTS.
// It is capped at 2 at most even if runtime inflight is higher, and drops to 1 if
// accountConcurrency is disabled or if the global runtime inflight is 1.
func resolvePerAccountLimit(store ConfigReader, accountConcurrency bool) int {
	if !accountConcurrency {
		return 1
	}
	if store != nil && store.RuntimeAccountMaxInflight() == 1 {
		return 1
	}
	return 2
}

// accountGate caps concurrent segment generations per account. The pool's
// in-flight limit is global configuration; this lets the TTS settings switch
// between two generations per account and one (the safer mode when accounts
// are being rate-limited).
type accountGate struct {
	mu       sync.Mutex
	gates    map[string]chan struct{}
	capacity int
}

func newAccountGate(capacity int) *accountGate {
	if capacity < 1 {
		capacity = 1
	}
	return &accountGate{gates: map[string]chan struct{}{}, capacity: capacity}
}

func (g *accountGate) acquire(ctx context.Context, accountID string) (func(), error) {
	key := strings.TrimSpace(accountID)
	if key == "" {
		key = "_"
	}
	g.mu.Lock()
	gate := g.gates[key]
	if gate == nil {
		gate = make(chan struct{}, g.capacity)
		g.gates[key] = gate
	}
	g.mu.Unlock()
	select {
	case gate <- struct{}{}:
		return func() { <-gate }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func writeOutputError(w http.ResponseWriter, outErr *assistantturn.OutputError) {
	status := outErr.Status
	if status < 400 || status > 599 {
		status = http.StatusBadGateway
	}
	shared.WriteOpenAIErrorWithCode(w, status, outErr.Message, outErr.Code)
}

// splitTTSText splits input on sentence boundaries so each segment stays under
// maxRunes. A single sentence longer than the limit falls back to clause
// boundaries and finally to a hard cut.
func splitTTSText(input string, maxRunes int) []string {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	if maxRunes <= 0 {
		maxRunes = maxTTSSegmentRunes
	}
	runes := []rune(input)
	if len(runes) <= maxRunes {
		return []string{input}
	}
	var segments []string
	for start := 0; start < len(runes); {
		end := start + maxRunes
		if end > len(runes) {
			end = len(runes)
		}
		cut := -1
		for i := end - 1; i >= start; i-- {
			if isTTSSentenceEnd(runes[i]) {
				cut = i + 1
				break
			}
		}
		if cut <= start {
			for i := end - 1; i >= start; i-- {
				if isTTSClauseEnd(runes[i]) {
					cut = i + 1
					break
				}
			}
		}
		if cut > start {
			// Absorb immediately following closing quotes and brackets into the current segment.
			for cut < len(runes) && isTTSClosingPunctuation(runes[cut]) {
				cut++
			}
		} else {
			cut = end
			if cut < len(runes) && !unicode.IsSpace(runes[cut]) {
				// Backward search for a whitespace to prevent bisecting a word.
				for s := cut - 1; s > start; s-- {
					if unicode.IsSpace(runes[s]) {
						cut = s + 1
						break
					}
				}
			}
		}
		if segment := strings.TrimSpace(string(runes[start:cut])); segment != "" {
			segments = append(segments, segment)
		}
		start = cut
		for start < len(runes) && unicode.IsSpace(runes[start]) {
			start++
		}
	}
	return segments
}

func isTTSClosingPunctuation(r rune) bool {
	switch r {
	case '”', '’', '"', '\'', '）', ')', '》', ']', '】', '』', '」':
		return true
	}
	return false
}

func isTTSSentenceEnd(r rune) bool {
	switch r {
	case '。', '！', '？', '；', '!', '?', ';', '．', '\n':
		return true
	}
	return false
}

func isTTSClauseEnd(r rune) bool {
	switch r {
	case '，', '、', ',', '：', ':', '—', '…':
		return true
	}
	return false
}
