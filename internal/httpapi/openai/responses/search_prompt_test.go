package responses

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResponsesUsesSearchFlagWithoutPromptInjection(t *testing.T) {
	for _, model := range []string{"deepseek-v4.1-flash", "deepseek-v4.1-flash-search", "deepseek-v4.1-flash-search-nothinking"} {
		t.Run(model, func(t *testing.T) {
			store, resolver := newDirectTokenResolver(t)
			ds := &responsesMarkerDS{toolCall: "answer"}
			h := &Handler{Store: store, Auth: resolver, DS: ds}
			req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"`+model+`","input":"Research this topic"}`))
			req.Header.Set("Authorization", "Bearer direct-token")
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			h.Responses(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("response failed: %d %s", rec.Code, rec.Body.String())
			}
			wantSearch := strings.Contains(model, "-search")
			prompt, _ := ds.payload["prompt"].(string)
			if ds.payload["search_enabled"] != wantSearch || strings.Contains(prompt, "Built-in web search protocol") {
				t.Fatal("upstream payload lost the search flag or injected a search prompt")
			}
			if strings.Contains(prompt, "<|EPSE|tool_calls>") || strings.Contains(prompt, "工具调用格式规范") {
				t.Fatal("native search injected client tool-call formatting")
			}
		})
	}
}
