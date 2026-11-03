package openai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestFileTokenUsageReachesChatAndResponsesStreamAndNonStream(t *testing.T) {
	for _, surface := range []string{"chat/completions", "responses"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%v", surface, stream), func(t *testing.T) {
				input := func(tokens int) int {
					ds := &inlineUploadDSStub{fileTokens: &tokens}
					h := &openAITestSurface{Store: mockOpenAIConfig{}, Auth: streamStatusAuthStub{}, DS: ds}
					router := chi.NewRouter()
					registerOpenAITestRoutes(router, h)
					key := "messages"
					if surface == "responses" {
						key = "input"
					}
					body := fmt.Sprintf(`{"model":"deepseek-v4.1-flash","%s":[{"role":"user","content":[{"type":"input_text","text":"describe"},{"type":"image_url","image_url":{"url":"data:image/png;base64,QUJDRA=="}}]}],"stream":%v}`, key, stream)
					req := httptest.NewRequest(http.MethodPost, "/v1/"+surface, strings.NewReader(body))
					req.Header.Set("Authorization", "Bearer direct-token")
					rec := httptest.NewRecorder()
					router.ServeHTTP(rec, req)
					if rec.Code != http.StatusOK {
						t.Fatalf("%d %s", rec.Code, rec.Body.String())
					}
					var usage map[string]any
					parse := func(raw string) {
						var obj map[string]any
						if json.Unmarshal([]byte(raw), &obj) != nil {
							return
						}
						if response, ok := obj["response"].(map[string]any); ok {
							obj = response
						}
						if candidate, ok := obj["usage"].(map[string]any); ok {
							usage = candidate
						}
					}
					if stream {
						for _, line := range strings.Split(rec.Body.String(), "\n") {
							if strings.HasPrefix(line, "data:") {
								parse(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
							}
						}
					} else {
						parse(rec.Body.String())
					}
					field := "prompt_tokens"
					if surface == "responses" {
						field = "input_tokens"
					}
					value, ok := usage[field].(float64)
					if !ok {
						t.Fatalf("usage missing: %s", rec.Body.String())
					}
					return int(value)
				}
				if delta := input(954) - input(0); delta != 954 {
					t.Fatalf("file usage delta = %d, want 954", delta)
				}
			})
		}
	}
}
