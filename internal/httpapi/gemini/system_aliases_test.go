package gemini

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
	"github.com/go-chi/chi/v5"
)

func TestSystemInstructionAliasesPreserveSourceAndBridge(t *testing.T) {
	system := map[string]any{"parts": []any{map[string]any{"text": "  SYSTEM_SENTINEL\r\n", "citations": []any{"SOURCE_SENTINEL"}}}}
	for _, keys := range [][]string{{"system_instruction"}, {"systemInstruction"}, {"systemInstruction", "system_instruction"}} {
		req := map[string]any{"contents": []any{map[string]any{"parts": []any{map[string]any{"text": "TASK"}}}}}
		for _, key := range keys {
			req[key] = system
		}
		before, _ := json.Marshal(req)
		standard, err := normalizeGeminiRequest(nil, "deepseek-v4.1-flash", req, false)
		if err != nil {
			t.Fatal(err)
		}
		chat, err := promptcompat.NormalizeOpenAIChatRequest(nil, standard.OpenAIChatRequest(req), "")
		if err != nil {
			t.Fatal(err)
		}
		for _, text := range []string{standard.FinalPrompt, chat.FinalPrompt, promptcompat.BuildOpenAIHistoryTranscript(standard.Messages)} {
			if strings.Count(text, "  SYSTEM_SENTINEL\r\n") != 1 || !strings.Contains(text, "SOURCE_SENTINEL") {
				t.Fatalf("system context changed: %s", text)
			}
		}
		fields := map[string]bool{}
		for _, field := range standard.InputContext {
			fields[field.Name] = true
		}
		for _, key := range keys {
			if !fields[key] {
				t.Fatalf("source field missing: %s", key)
			}
		}
		after, _ := json.Marshal(req)
		if string(before) != string(after) {
			t.Fatal("normalization mutated caller input")
		}
	}
}

func TestSystemInstructionAliasConflictsRejectBeforeProxy(t *testing.T) {
	for _, snake := range []string{`null`, `"DIFFERENT"`, `42`} {
		capture := &geminiOpenAISuccessStub{}
		router := chi.NewRouter()
		RegisterRoutes(router, &Handler{OpenAI: capture})
		rec := httptest.NewRecorder()
		body := `{"systemInstruction":"SYSTEM","system_instruction":` + snake + `,"contents":[{"parts":[{"text":"TASK"}]}]}`
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1beta/models/deepseek-v4.1-flash:generateContent?__stream_prepare=1", strings.NewReader(body)))
		if rec.Code != http.StatusBadRequest || capture.seenReq != nil {
			t.Fatalf("conflict reached proxy: %d %s", rec.Code, rec.Body.String())
		}
	}
}
