package audio

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"ds2api/internal/auth"
	dsclient "ds2api/internal/deepseek/client"
)

// stubAuth satisfies shared.AuthResolver without touching the account pool.
type stubAuth struct{}

func (stubAuth) Determine(*http.Request) (*auth.RequestAuth, error) {
	return &auth.RequestAuth{UseConfigToken: true, AccountID: "test", DeepSeekToken: "token"}, nil
}
func (stubAuth) DetermineTry(*http.Request) (*auth.RequestAuth, error) {
	return &auth.RequestAuth{UseConfigToken: true, AccountID: "test", DeepSeekToken: "token"}, nil
}
func (stubAuth) DetermineCaller(*http.Request) (*auth.RequestAuth, error) {
	return &auth.RequestAuth{}, nil
}
func (stubAuth) CredentialFlagForRequest(*http.Request, string) bool { return false }
func (stubAuth) Release(*auth.RequestAuth)                           {}
func (stubAuth) SetAccountMutedUntil(*auth.RequestAuth, float64)     {}
func (stubAuth) SetAccountBanned(*auth.RequestAuth, string)          {}

// stubDS embeds the interface so unimplemented methods are only reached if a
// test drives a path past validation; the validation tests never do.
type stubDS struct{ DeepSeekCaller }

func newTestHandler() *Handler {
	return &Handler{Auth: stubAuth{}, DS: &stubDS{}}
}

func TestSpeechValidation(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{name: "empty input", body: `{"input":""}`, want: http.StatusBadRequest},
		{name: "missing input", body: `{}`, want: http.StatusBadRequest},
		{name: "oversized input", body: `{"input":"` + strings.Repeat("a", maxTTSInputRunes+1) + `"}`, want: http.StatusBadRequest},
		{name: "unknown voice", body: `{"input":"hi","voice":"nope"}`, want: http.StatusBadRequest},
		{name: "unsupported format", body: `{"input":"hi","response_format":"mp3"}`, want: http.StatusBadRequest},
		{name: "unsupported speed", body: `{"input":"hi","speed":2}`, want: http.StatusBadRequest},
		{name: "explicit zero speed", body: `{"input":"hi","speed":0}`, want: http.StatusBadRequest},
		{name: "unknown model", body: `{"input":"hi","model":"no-such-model"}`, want: http.StatusBadRequest},
		{name: "invalid json", body: `{`, want: http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()
			newTestHandler().Speech(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status=%d want %d body=%s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestNormalizeVoice(t *testing.T) {
	tests := map[string]string{
		"":        "mira",
		"mira":    "mira",
		"echo":    "echo",
		"stella":  "stella",
		"tide":    "tide",
		"MIRA":    "mira",
		"贝壳":      "mira",
		"白浪":      "echo",
		"海星":      "stella",
		"暗潮":      "tide",
		"alloy":   "mira",
		"shimmer": "mira",
	}
	for input, want := range tests {
		got, err := normalizeVoice(input)
		if err != nil {
			t.Errorf("normalizeVoice(%q): %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("normalizeVoice(%q)=%q want %q", input, got, want)
		}
	}
	if _, err := normalizeVoice("unknown"); err == nil {
		t.Fatal("expected an error for an unknown voice")
	}
}

func TestNormalizeFormat(t *testing.T) {
	for _, input := range []string{"", "opus", "ogg", "ogg_opus", "OPUS"} {
		got, err := normalizeFormat(input)
		if err != nil {
			t.Errorf("normalizeFormat(%q): %v", input, err)
			continue
		}
		if got != "opus" {
			t.Errorf("normalizeFormat(%q)=%q want opus", input, got)
		}
	}
	for _, input := range []string{"wav", "wave", "WAV"} {
		got, err := normalizeFormat(input)
		if err != nil {
			t.Errorf("normalizeFormat(%q): %v", input, err)
			continue
		}
		if got != "wav" {
			t.Errorf("normalizeFormat(%q)=%q want wav", input, got)
		}
	}
	if _, err := normalizeFormat("mp3"); err == nil {
		t.Fatal("expected an error for mp3")
	}
}

func TestBuildRecitePromptIncludesInput(t *testing.T) {
	prompt := buildRecitePrompt("你好，世界")
	if !strings.Contains(prompt, "你好，世界") {
		t.Fatalf("prompt does not contain the input: %q", prompt)
	}
	if !strings.Contains(prompt, "原样复述") {
		t.Fatalf("prompt is missing the verbatim instruction: %q", prompt)
	}
}

func TestResolveModelAcceptsOpenAITTSNames(t *testing.T) {
	h := &Handler{}
	got, err := h.resolveModel("tts-1")
	if err != nil {
		t.Fatalf("resolveModel(tts-1): %v", err)
	}
	if got != defaultTTSModel {
		t.Fatalf("resolveModel(tts-1)=%q want %q", got, defaultTTSModel)
	}
}

func TestSplitTTSText(t *testing.T) {
	if got := splitTTSText("你好，世界。", 160); len(got) != 1 || got[0] != "你好，世界。" {
		t.Fatalf("short text must stay one segment, got %#v", got)
	}
	if got := splitTTSText("   ", 160); len(got) != 0 {
		t.Fatalf("blank input must produce no segments, got %#v", got)
	}

	long := strings.Repeat("这是一句话。", 50)
	segments := splitTTSText(long, 100)
	if len(segments) < 2 {
		t.Fatalf("long text must be split, got %d segments", len(segments))
	}
	for _, segment := range segments {
		if len([]rune(segment)) > 100 {
			t.Fatalf("segment exceeds the limit: %d runes", len([]rune(segment)))
		}
	}

	hard := strings.Repeat("字", 250)
	segments = splitTTSText(hard, 100)
	if len(segments) != 3 {
		t.Fatalf("hard cut segments=%d want 3", len(segments))
	}
	for _, segment := range segments {
		if len([]rune(segment)) > 100 {
			t.Fatalf("hard-cut segment exceeds the limit: %d runes", len([]rune(segment)))
		}
	}

	// Test quote absorption
	withQuote := "他说：“你好！”下一句。"
	absorbed := splitTTSText(withQuote, 8)
	if len(absorbed) < 2 || !strings.HasSuffix(absorbed[0], "！”") {
		t.Fatalf("closing quote must be absorbed into first segment, got %#v", absorbed)
	}

	// Test English word boundary fallback
	enText := "The quick brown fox jumps over the lazy dog"
	enSegs := splitTTSText(enText, 15)
	for _, seg := range enSegs {
		if strings.HasPrefix(seg, "ox") || strings.HasPrefix(seg, "og") {
			t.Fatalf("words must not be cut midway: got %#v", enSegs)
		}
	}
}

func TestSpeechGETValidation(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/audio/speech", nil)
	rec := httptest.NewRecorder()
	newTestHandler().SpeechGET(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 body=%s", rec.Code, rec.Body.String())
	}
}

type stubDisabledStore struct{}

func (stubDisabledStore) ModelAliases() map[string]string    { return nil }
func (stubDisabledStore) ModelAlias(string) (string, bool)   { return "", false }
func (stubDisabledStore) TTSEnabled() bool                   { return false }
func (stubDisabledStore) TTSSegmentChars() int               { return 160 }
func (stubDisabledStore) TTSMultiAccountEnabled() bool       { return true }
func (stubDisabledStore) TTSAccountConcurrencyEnabled() bool { return true }
func (stubDisabledStore) RuntimeAccountMaxInflight() int     { return 2 }

func TestSpeechDisabledReturnsServiceUnavailable(t *testing.T) {
	h := &Handler{
		Auth:  stubAuth{},
		DS:    &stubDS{},
		Store: stubDisabledStore{},
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{"input":"hello"}`))
	rec := httptest.NewRecorder()
	h.Speech(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want 503 body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "tts_disabled") {
		t.Fatalf("expected tts_disabled error code in body: %s", rec.Body.String())
	}
}

type stubEnabledStore struct{ stubDisabledStore }

func (stubEnabledStore) TTSEnabled() bool { return true }

type stubUnauthorizedAuth struct{ stubAuth }

func (stubUnauthorizedAuth) Determine(*http.Request) (*auth.RequestAuth, error) {
	return nil, auth.ErrUnauthorized
}

// DetermineCaller mirrors the real resolver: a request with no credential is
// rejected before any account is acquired.
func (stubUnauthorizedAuth) DetermineCaller(*http.Request) (*auth.RequestAuth, error) {
	return nil, auth.ErrUnauthorized
}

func TestSpeechUnauthorizedReturns401(t *testing.T) {
	h := &Handler{
		Auth:  stubUnauthorizedAuth{},
		DS:    &stubDS{},
		Store: stubEnabledStore{},
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{"input":"hello"}`))
	rec := httptest.NewRecorder()
	h.Speech(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401 body=%s", rec.Code, rec.Body.String())
	}
}

type stubInflightStore struct {
	stubEnabledStore
	inflight int
}

func (s stubInflightStore) RuntimeAccountMaxInflight() int { return s.inflight }

func TestResolvePerAccountLimit(t *testing.T) {
	// When account concurrency is turned off, always 1.
	if got := resolvePerAccountLimit(stubInflightStore{inflight: 4}, false); got != 1 {
		t.Fatalf("got %d want 1 when accountConcurrency is false", got)
	}
	// When global runtime inflight is 1, drops to 1.
	if got := resolvePerAccountLimit(stubInflightStore{inflight: 1}, true); got != 1 {
		t.Fatalf("got %d want 1 when runtime inflight is 1", got)
	}
	// Normal case: capped at 2.
	if got := resolvePerAccountLimit(stubInflightStore{inflight: 2}, true); got != 2 {
		t.Fatalf("got %d want 2 when runtime inflight is 2", got)
	}
	// Higher global runtime inflight (e.g. 4, 10): must stay capped at 2.
	if got := resolvePerAccountLimit(stubInflightStore{inflight: 4}, true); got != 2 {
		t.Fatalf("got %d want 2 when runtime inflight is 4", got)
	}
	if got := resolvePerAccountLimit(stubInflightStore{inflight: 100}, true); got != 2 {
		t.Fatalf("got %d want 2 when runtime inflight is 100", got)
	}
	// Nil store falls back to 2 when accountConcurrency is true.
	if got := resolvePerAccountLimit(nil, true); got != 2 {
		t.Fatalf("got %d want 2 when store is nil", got)
	}
}

type stubEmptyReciteDS struct{ stubDS }

func (stubEmptyReciteDS) CreateSession(context.Context, *auth.RequestAuth, int) (string, error) {
	return "session-1", nil
}

func (stubEmptyReciteDS) GetPow(context.Context, *auth.RequestAuth, int) (string, error) {
	return "pow", nil
}

func (stubEmptyReciteDS) CallCompletion(context.Context, *auth.RequestAuth, map[string]any, string, int) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("data: {\"response_message_id\":1,\"v\":\"   \"}\n\ndata: [DONE]\n\n")),
	}, nil
}

func (stubEmptyReciteDS) DeleteSessionForToken(context.Context, string, string) (*dsclient.DeleteSessionResult, error) {
	return nil, nil
}

func TestReciteEmptyTextReturnsError(t *testing.T) {
	h := &Handler{
		Auth:  stubAuth{},
		DS:    &stubEmptyReciteDS{},
		Store: stubEnabledStore{},
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{"input":"hello"}`))
	rec := httptest.NewRecorder()
	h.Speech(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatalf("expected error for empty recitation, got 200")
	}
	if !strings.Contains(rec.Body.String(), "tts_empty_recitation") && !strings.Contains(rec.Body.String(), "upstream_unavailable") {
		t.Fatalf("expected error code for empty recitation in body: %s", rec.Body.String())
	}
}

type stubSegmentStore struct {
	stubEnabledStore
	segmentChars int
}

func (s stubSegmentStore) TTSSegmentChars() int { return s.segmentChars }

// countingAuth records how many times the pool was asked for an account.
type countingAuth struct {
	stubAuth
	mu         sync.Mutex
	determines int
	accounts   int
}

func (c *countingAuth) Determine(*http.Request) (*auth.RequestAuth, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.determines++
	return &auth.RequestAuth{UseConfigToken: true, AccountID: "test", DeepSeekToken: "token"}, nil
}

func (c *countingAuth) DetermineTry(req *http.Request) (*auth.RequestAuth, error) {
	return c.Determine(req)
}

func (c *countingAuth) SchedulableAccounts() int { return c.accounts }

func (c *countingAuth) determineCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.determines
}

type stubSuccessDS struct{ stubDS }

func (stubSuccessDS) CreateSession(context.Context, *auth.RequestAuth, int) (string, error) {
	return "session-1", nil
}

func (stubSuccessDS) GetPow(context.Context, *auth.RequestAuth, int) (string, error) {
	return "pow", nil
}

func (stubSuccessDS) CallCompletion(context.Context, *auth.RequestAuth, map[string]any, string, int) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("data: {\"response_message_id\":7,\"v\":\"朗读内容\"}\n\ndata: [DONE]\n\n")),
	}, nil
}

func (stubSuccessDS) SynthesizeSpeech(context.Context, *auth.RequestAuth, string, int, dsclient.TTSOptions) (*dsclient.TTSAudio, error) {
	return &dsclient.TTSAudio{
		Frames:  [][]byte{{0x6b, 0x01, 0x02}, {0x6b, 0x03, 0x04}},
		VoiceID: "mira",
		AudioID: "audio-1",
	}, nil
}

func (stubSuccessDS) DeleteSessionForToken(context.Context, string, string) (*dsclient.DeleteSessionResult, error) {
	return nil, nil
}

// TestSpeechSynthesisAcquiresOneAccountPerWorker pins the worker-level account
// acquisition: a multi-segment request takes one account per worker, not one
// per segment. Per-minute and 24h account rate limits count every acquire, so
// per-segment acquisition made one long request exhaust its own budget and fail
// mid-flight. Credential validation no longer acquires an account, so the count
// is exactly the worker count.
func TestSpeechSynthesisAcquiresOneAccountPerWorker(t *testing.T) {
	authStub := &countingAuth{accounts: 1}
	h := &Handler{
		Auth:  authStub,
		DS:    stubSuccessDS{},
		Store: stubSegmentStore{stubEnabledStore{}, 30},
	}
	// 75 runes at the 30-rune segment floor split into 3 segments; with one
	// schedulable account and 2-segment per-account concurrency that means 2
	// workers, so exactly 2 acquires.
	input := strings.Repeat("第一句话。", 15)
	req := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{"input":"`+input+`"}`))
	rec := httptest.NewRecorder()
	h.Speech(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-Ds2-Tts-Segments"); got != "3" {
		t.Fatalf("X-Ds2-Tts-Segments=%q want 3", got)
	}
	if got := rec.Header().Get("X-Ds2-Tts-Frames"); got != strconv.Itoa(3*2) {
		t.Fatalf("X-Ds2-Tts-Frames=%q want 6", got)
	}
	if got := authStub.determineCount(); got != 2 {
		t.Fatalf("Determine calls=%d want 2 (one per worker)", got)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "audio/ogg" {
		t.Fatalf("Content-Type=%q want audio/ogg", ct)
	}
}

type stubSingleAccountStore struct{ stubSegmentStore }

func (stubSingleAccountStore) TTSMultiAccountEnabled() bool { return false }

// TestSpeechSynthesisReusesSingleAccountSeed pins the single-account path: the
// account acquired up front is handed to the first worker instead of being
// released and re-acquired. Releasing first opened a window where another
// request could take the pinned account, and re-acquiring also spent a second
// rate-limit token on it.
func TestSpeechSynthesisReusesSingleAccountSeed(t *testing.T) {
	authStub := &countingAuth{accounts: 1}
	h := &Handler{
		Auth:  authStub,
		DS:    stubSuccessDS{},
		Store: stubSingleAccountStore{stubSegmentStore{stubEnabledStore{}, 30}},
	}
	input := strings.Repeat("第一句话。", 15)
	req := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{"input":"`+input+`"}`))
	rec := httptest.NewRecorder()
	h.Speech(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	// One acquire for the seed plus one for the second worker; the first worker
	// reuses the seed, so three segments must not produce four acquires.
	if got := authStub.determineCount(); got != 2 {
		t.Fatalf("Determine calls=%d want 2 (1 seed reused + 1 second worker)", got)
	}
}

func TestAudioQueryCredential(t *testing.T) {
	// ?token= maps onto the Authorization header when no header credential exists.
	req := httptest.NewRequest(http.MethodGet, "/v1/audio/speech?input=hi&token=abc", nil)
	rewritten := audioQueryCredential(req)
	if got := rewritten.Header.Get("Authorization"); got != "Bearer abc" {
		t.Fatalf("Authorization=%q want Bearer abc", got)
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Fatalf("original request must stay untouched, got %q", got)
	}

	// A header credential wins: the query token is ignored.
	req = httptest.NewRequest(http.MethodGet, "/v1/audio/speech?input=hi&token=abc", nil)
	req.Header.Set("x-api-key", "header-key")
	rewritten = audioQueryCredential(req)
	if got := rewritten.Header.Get("Authorization"); got != "" {
		t.Fatalf("Authorization=%q want empty when a header credential is present", got)
	}

	// No token: the request is returned as-is.
	req = httptest.NewRequest(http.MethodGet, "/v1/audio/speech?input=hi", nil)
	if rewritten = audioQueryCredential(req); rewritten != req {
		t.Fatal("request without ?token= must be returned unchanged")
	}
}

// stubHeaderOnlyAuth accepts only header-based credentials, mirroring the
// auth layer's refusal to read ?token= from query strings.
type stubHeaderOnlyAuth struct{ stubAuth }

func (stubHeaderOnlyAuth) Determine(req *http.Request) (*auth.RequestAuth, error) {
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(req.Header.Get("Authorization"))), "bearer ") {
		return nil, auth.ErrUnauthorized
	}
	return &auth.RequestAuth{UseConfigToken: true, AccountID: "test", DeepSeekToken: "token"}, nil
}

func (stubHeaderOnlyAuth) DetermineCaller(req *http.Request) (*auth.RequestAuth, error) {
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(req.Header.Get("Authorization"))), "bearer ") {
		return nil, auth.ErrUnauthorized
	}
	return &auth.RequestAuth{}, nil
}

// TestSpeechGETQueryTokenAuthenticates pins the reader-app contract: ?token=
// authenticates on this endpoint (mapped to the header by the handler), while a
// request with no credential at all still fails with 401.
func TestSpeechGETQueryTokenAuthenticates(t *testing.T) {
	h := &Handler{
		Auth:  stubHeaderOnlyAuth{},
		DS:    stubSuccessDS{},
		Store: stubEnabledStore{},
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/audio/speech?input=hello&token=key-1&response_format=opus", nil)
	rec := httptest.NewRecorder()
	h.SpeechGET(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/audio/speech?input=hello", nil)
	rec = httptest.NewRecorder()
	h.SpeechGET(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401 without credentials", rec.Code)
	}
}
