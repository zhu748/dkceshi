package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"ds2api/internal/auth"
	"ds2api/internal/httpapi/openai/files"
	"ds2api/internal/promptcompat"
)

func TestGeminiProxyKeepsNestedToolFilesUntilSharedUpload(t *testing.T) {
	part := map[string]any{"type": "document", "title": "NESTED_TITLE", "source": map[string]any{"type": "content", "content": []any{
		map[string]any{"type": "text", "text": "  NESTED_CONTEXT\r\n"},
		map[string]any{"inlineData": map[string]any{"mimeType": "image/png", "data": "QUJDRA==", "displayName": "NESTED_IMAGE"}},
	}}}
	data, _ := json.Marshal(map[string]any{"contents": []any{map[string]any{"role": "user", "parts": []any{map[string]any{"functionResponse": map[string]any{"id": "external-call", "name": "read", "response": map[string]any{"text": "BUSINESS_RESULT"}, "parts": []any{part}}}}}}})
	capture := &geminiOpenAISuccessStub{}
	h := &Handler{OpenAI: capture}
	router := chi.NewRouter()
	RegisterRoutes(router, h)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1beta/models/deepseek-v4.1-flash:generateContent?__stream_prepare=1", strings.NewReader(string(data))))
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	uploader := &contextMediaUploader{}
	if err := files.PreprocessInlineFiles(context.Background(), &auth.RequestAuth{}, capture.seenReq, uploader, "deepseek-v4.1-flash", false); err != nil {
		t.Fatal(err)
	}
	if len(uploader.uploads) != 1 || string(uploader.uploads[0].Data) != "ABCD" {
		t.Fatalf("nested media was sanitized before upload: %+v", uploader.uploads)
	}
	standard, err := promptcompat.NormalizeOpenAIChatRequest(nil, capture.seenReq, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{standard.FinalPrompt, promptcompat.BuildOpenAIHistoryTranscript(standard.Messages)} {
		for _, want := range []string{"NESTED_TITLE", "NESTED_IMAGE", "NESTED_CONTEXT", "BUSINESS_RESULT", "file-media-1"} {
			if !strings.Contains(text, want) {
				t.Fatalf("nested tool context lost %s", want)
			}
		}
		if strings.Contains(text, "QUJDRA==") {
			t.Fatal("nested bytes leaked into chat text")
		}
	}
}
