package chat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
)

func TestSearchToolFailureAfterWhitespaceCanRegenerate(t *testing.T) {
	tools := []any{map[string]any{"name": "web_search", "parameters": map[string]any{
		"type": "object", "required": []any{"queries"},
		"properties": map[string]any{"queries": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}},
	}}}
	for _, name := range []string{"web_search", "search"} {
		for _, prefix := range []string{"\n\n", " \t\r\n\u3000", "Searching now."} {
			t.Run(name+"/"+prefix, func(t *testing.T) {
				rec := httptest.NewRecorder()
				runtime := newChatStreamRuntime(rec, http.NewResponseController(rec), true, "id", 1, "model", "prompt", false, true, true, []string{"web_search"}, tools, promptcompat.ToolChoicePolicy{Mode: promptcompat.ToolChoiceAuto, RestrictAllowed: true, Allowed: map[string]struct{}{"web_search": {}}}, true, true)
				runtime.toolSieve.Marker = "ZBBZUX"
				content := func(parts ...string) *http.Response {
					var lines []string
					for _, part := range parts {
						data, err := json.Marshal(map[string]any{"p": "response/content", "v": part})
						if err != nil {
							t.Fatal(err)
						}
						lines = append(lines, "data: "+string(data))
					}
					return makeOpenAISSEHTTPResponse(append(lines, "data: [DONE]")...)
				}
				bad := `<|ZBBZUX|tool_calls><|ZBBZUX|invoke name="` + name + `"><|ZBBZUX|parameter name="queries"><![CDATA[first||second]]></|ZBBZUX|parameter></|ZBBZUX|invoke></|ZBBZUX|tool_calls>`
				req := httptest.NewRequest(http.MethodPost, "/", nil)
				result := (&Handler{}).consumeChatStreamAttempt(req, content(prefix, bad), runtime, "text", false, nil, true)
				delivered := strings.TrimSpace(prefix) != ""
				if result.Terminal != delivered || result.Retryable == delivered {
					t.Fatalf("incorrect delivery boundary: result=%+v code=%s body=%s", result, runtime.finalErrorCode, rec.Body.String())
				}
				if delivered {
					return
				}
				runtime.resetForRegeneration()
				good := `<|ZBBZUX|tool_calls><|ZBBZUX|invoke name="web_search"><|ZBBZUX|parameter name="queries">["first","second"]</|ZBBZUX|parameter></|ZBBZUX|invoke></|ZBBZUX|tool_calls>`
				result = (&Handler{}).consumeChatStreamAttempt(req, content(good), runtime, "text", false, nil, true)
				body := rec.Body.String()
				if !result.Terminal || strings.Contains(body, `"error"`) || strings.Contains(body, "ZBBZUX") || strings.Count(body, `"finish_reason":"tool_calls"`) != 1 || !strings.Contains(body, `\"queries\":[\"first\",\"second\"]`) {
					t.Fatalf("invalid attempt escaped or valid retry lost: %+v body=%s", result, body)
				}
			})
		}
	}
}
