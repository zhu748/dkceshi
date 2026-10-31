package gemini

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"ds2api/internal/auth"
	"github.com/go-chi/chi/v5"
)

func TestNoIdleAccountReturnsRateLimit(t *testing.T) {
	h := &Handler{Store: testGeminiConfig{}, Auth: testGeminiAuth{err: auth.ErrNoIdleAccount}, DS: &testGeminiDS{}}
	r := chi.NewRouter()
	RegisterRoutes(r, h)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-flash:generateContent", strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("quota exhaustion became HTTP %d rather than 429: %s", w.Code, w.Body.String())
	}
}

func TestUnknownPartTruncationPreservesUTF8(t *testing.T) {
	s := formatGeminiUnknownPartForPrompt(map[string]any{"unknown": strings.Repeat("中", 400)})
	if !utf8.ValidString(s) {
		t.Error("byte truncation splits a UTF-8 character in normalized Gemini prompt")
	}
}
