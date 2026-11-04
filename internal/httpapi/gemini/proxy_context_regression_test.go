package gemini

import (
	"ds2api/internal/promptcompat"
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGeminiProxyKeepsParallelResultsSchemaAndAllowedTools(t *testing.T) {
	capture := &geminiOpenAISuccessStub{}
	h := &Handler{OpenAI: capture}
	body := `{"contents":[{"role":"model","parts":[{"functionCall":{"id":"a","name":"check","args":{}}},{"functionCall":{"id":"b","name":"check","args":{}}}]},{"role":"user","parts":[{"functionResponse":{"id":"b","name":"check","response":{"for":"b"}}},{"functionResponse":{"id":"a","name":"check","response":{"for":"a"}}}]}],"tools":[{"functionDeclarations":[{"name":"check","parameters":{"type":"OBJECT","properties":{"id":{"type":"INTEGER"}}}},{"name":"other"}]}],"toolConfig":{"functionCallingConfig":{"mode":"ANY","allowedFunctionNames":["check"]}}}`
	rec := httptest.NewRecorder()
	router := chi.NewRouter()
	RegisterRoutes(router, h)
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1beta/models/deepseek-v4.1-flash:generateContent?__stream_prepare=1", strings.NewReader(body)))
	if rec.Code != 200 {
		t.Fatalf("proxy failed: %d %s", rec.Code, rec.Body.String())
	}
	std, err := promptcompat.NormalizeOpenAIChatRequest(nil, capture.seenReq, "")
	if err != nil {
		t.Fatal(err)
	}
	if !std.ToolChoice.IsRequired() || std.ToolChoice.Allows("other") {
		t.Fatalf("tool policy lost: %+v", std.ToolChoice)
	}
	var ids []string
	for _, raw := range std.Messages {
		m := raw.(map[string]any)
		if m["role"] == "tool" {
			ids = append(ids, m["tool_call_id"].(string))
		}
	}
	if len(ids) != 2 || ids[0] != "b" || ids[1] != "a" {
		t.Fatalf("explicit result IDs changed: %v", ids)
	}
}

func TestGeminiSignatureDoesNotMoveAnswerIntoReasoning(t *testing.T) {
	for _, part := range []map[string]any{{"text": "ANSWER", "thoughtSignature": "opaque"}, {"text": "ANSWER", "thought": false, "thoughtSignature": "opaque"}} {
		got := geminiMessagesFromRequest(map[string]any{"contents": []any{map[string]any{"role": "model", "parts": []any{part}}}})
		if len(got) != 1 || got[0].(map[string]any)["content"] != "ANSWER" {
			t.Fatalf("answer became reasoning: %#v", got)
		}
	}
}
