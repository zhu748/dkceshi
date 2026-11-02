package responses

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
)

func TestSearchToolFailureAfterProseCanRegenerate(t *testing.T) {
	tools := []any{map[string]any{"name": "web_search", "parameters": map[string]any{
		"type": "object", "required": []any{"queries"},
		"properties": map[string]any{"queries": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}},
	}}}
	for _, prefix := range []string{"\n\n", " \t\u3000", "Searching now."} {
		t.Run(prefix, func(t *testing.T) {
			rec := httptest.NewRecorder()
			runtime := newResponsesStreamRuntime(rec, http.NewResponseController(rec), true, "id", "model", "prompt", false, true, true, []string{"web_search"}, tools, true, true, promptcompat.DefaultToolChoicePolicy(), "", nil, nil)
			content := func(parts ...string) *http.Response {
				var lines []string
				for _, part := range parts {
					data, err := json.Marshal(map[string]any{"p": "response/content", "v": part})
					if err != nil {
						t.Fatal(err)
					}
					lines = append(lines, "data: "+string(data))
				}
				return makeResponsesOpenAISSEHTTPResponse(append(lines, "data: [DONE]")...)
			}
			bad := `<tool_calls><invoke name="web_search"><parameter name="queries"><![CDATA[first||second]]></parameter></invoke></tool_calls>`
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			result := (&Handler{}).consumeResponsesStreamAttempt(req, content(prefix, bad), runtime, "text", false, true)
			if result.Terminal || !result.Retryable || rec.Body.Len() != 0 {
				t.Fatalf("incorrect delivery boundary: result=%+v code=%s body=%s", result, runtime.finalErrorCode, rec.Body.String())
			}
			runtime.resetForRegeneration()
			good := strings.Replace(bad, "<![CDATA[first||second]]>", `["first","second"]`, 1)
			result = (&Handler{}).consumeResponsesStreamAttempt(req, content(good), runtime, "text", false, true)
			body := rec.Body.String()
			if !result.Terminal || strings.Contains(body, "response.failed") || strings.Count(body, "event: response.completed\n") != 1 || strings.Contains(body, "first||second") {
				t.Fatalf("invalid attempt escaped or retry failed: %+v body=%s", result, body)
			}
		})
	}
}
