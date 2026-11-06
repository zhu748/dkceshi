package gemini

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
	"github.com/go-chi/chi/v5"
)

func TestGeminiProxyRejectsNativeToolConflictBeforeForwarding(t *testing.T) {
	capture := &geminiOpenAISuccessStub{}
	h := &Handler{OpenAI: capture}
	router := chi.NewRouter()
	RegisterRoutes(router, h)
	for _, part := range []string{
		`{"functionCall":{"name":"read","id":"a","callId":"b","args":{}}}`,
		`{"functionResponse":{"id":"a","response":null,"output":"OVERWRITTEN"}}`,
		`{"functionResponse":{"id":"a","response":"ok","parts":"LOST"}}`,
	} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1beta/models/deepseek-v4.1-flash:generateContent?__stream_prepare=1", strings.NewReader(`{"contents":[{"role":"model","parts":[`+part+`]}]}`)))
		if rec.Code != http.StatusBadRequest || capture.seenReq != nil {
			t.Fatalf("conflicting native context forwarded: %d %s", rec.Code, rec.Body.String())
		}
	}
}

func TestGeminiToolResultRetainsExplicitNullWithOrWithoutMedia(t *testing.T) {
	for _, parts := range [][]any{nil, {map[string]any{"text": "MEDIA_CONTEXT"}}} {
		response := map[string]any{"name": "read", "response": nil}
		if parts != nil {
			response["parts"] = parts
		}
		norm, err := normalizeGeminiRequest(nil, "deepseek-v4.1-flash", map[string]any{"contents": []any{map[string]any{"parts": []any{map[string]any{"functionResponse": response}}}}}, false)
		if err != nil {
			t.Fatal(err)
		}
		for _, text := range []string{norm.FinalPrompt, promptcompat.BuildOpenAIHistoryTranscript(norm.Messages)} {
			if !strings.Contains(text, "[Tool]:null") || parts != nil && !strings.Contains(text, "MEDIA_CONTEXT") {
				t.Fatalf("explicit null result lost: %s", text)
			}
		}
	}
}
