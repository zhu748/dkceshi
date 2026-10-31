package openai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestVercelPrepareUsesSearchFlagWithoutPromptInjection(t *testing.T) {
	t.Setenv("VERCEL", "1")
	t.Setenv("DS2API_VERCEL_INTERNAL_SECRET", "stream-secret")
	for _, model := range []string{"deepseek-v4.1-flash", "deepseek-v4.1-flash-search", "deepseek-v4.1-flash-search-nothinking"} {
		t.Run(model, func(t *testing.T) {
			h := &openAITestSurface{Store: mockOpenAIConfig{}, Auth: streamStatusAuthStub{}, DS: &inlineUploadDSStub{}}
			router := chi.NewRouter()
			registerOpenAITestRoutes(router, h)
			body := `{"model":"` + model + `","messages":[{"role":"user","content":"Research this topic"}],"stream":true}`
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?__stream_prepare=1", strings.NewReader(body))
			req.Header.Set("Authorization", "Bearer direct-token")
			req.Header.Set("X-Ds2-Internal-Token", "stream-secret")
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("prepare failed: %d %s", rec.Code, rec.Body.String())
			}
			var out struct {
				FinalPrompt string         `json:"final_prompt"`
				Search      bool           `json:"search_enabled"`
				ToolNames   []string       `json:"tool_names"`
				Payload     map[string]any `json:"payload"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			search := strings.Contains(model, "-search")
			if strings.Contains(out.FinalPrompt, "Built-in web search protocol") || out.Search != search || out.Payload["search_enabled"] != search {
				t.Fatal("Node prepare lost model-based search gating")
			}
			if out.Payload["prompt"] != out.FinalPrompt || len(out.ToolNames) != 0 {
				t.Fatal("Node must receive the prepared prompt without adding client tools")
			}
		})
	}
}
