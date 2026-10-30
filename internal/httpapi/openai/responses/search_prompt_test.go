package responses

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResponsesSearchPromptReachesUpstreamPayload(t *testing.T) {
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
			wantCount := 0
			if wantSearch {
				wantCount = 1
			}
			prompt, _ := ds.payload["prompt"].(string)
			if ds.payload["search_enabled"] != wantSearch || strings.Count(prompt, "Built-in web search protocol") != wantCount {
				t.Fatal("upstream payload lost model-based search instructions")
			}
			if strings.Contains(prompt, "<|EPSE|tool_calls>") || strings.Contains(prompt, "工具调用格式规范") {
				t.Fatal("native search injected client tool-call formatting")
			}
		})
	}
}
