package chat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
)

func TestAppNativeSearchStaysInReasoningWithClientWebTools(t *testing.T) {
	tools := []any{map[string]any{"name": "web_search", "parameters": map[string]any{
		"type": "object", "required": []any{"queries"},
		"properties": map[string]any{"queries": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}},
	}}}
	for _, thinking := range []bool{true, false} {
		name, initialType := "hidden_reasoning", "text"
		if thinking {
			name, initialType = "visible_reasoning", "thinking"
		}
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			runtime := newChatStreamRuntime(rec, http.NewResponseController(rec), true, "id", 1, "model", "prompt", thinking, true, true, []string{"web_search"}, tools, promptcompat.DefaultToolChoicePolicy(), true, true)
			runtime.toolCallRepair = func(context.Context, string) (string, error) {
				t.Fatal("App search must not invoke client tool-call repair")
				return "", nil
			}
			resp := makeOpenAISSEHTTPResponse(
				`data: {"p":"response/fragments","o":"APPEND","v":[{"type":"THINK","content":"Need current sources."}]}`,
				`data: {"p":"response/fragments","o":"APPEND","v":[{"type":"TOOL_SEARCH","content":"Found 16 pages.","queries":["query"],"results":[]}]}`,
				`data: {"p":"response/fragments/-1","v":[{"p":"status","v":"FINISHED"}]}`,
				`data: {"p":"response/fragments","o":"APPEND","v":[{"type":"TOOL_OPEN","content":"Read 4 pages."}]}`,
				`data: {"p":"response/fragments","o":"APPEND","v":[{"type":"RESPONSE","content":"Answer from native search."}]}`,
				`data: {"p":"response/status","v":"FINISHED"}`,
			)
			result := (&Handler{}).consumeChatStreamAttempt(httptest.NewRequest(http.MethodPost, "/", nil), resp, runtime, initialType, thinking, nil, true)
			frames, done := parseSSEDataFrames(t, rec.Body.String())
			var content, reasoning strings.Builder
			for _, frame := range frames {
				if frame["error"] != nil {
					t.Fatalf("native search returned an error: %s", rec.Body.String())
				}
				choices, _ := frame["choices"].([]any)
				for _, raw := range choices {
					choice, _ := raw.(map[string]any)
					delta, _ := choice["delta"].(map[string]any)
					if delta["tool_calls"] != nil {
						t.Fatal("native search became a client tool call")
					}
					text, _ := delta["content"].(string)
					content.WriteString(text)
					text, _ = delta["reasoning_content"].(string)
					reasoning.WriteString(text)
				}
			}
			if !result.Terminal || !done || content.String() != "Answer from native search." || strings.Contains(reasoning.String(), "Found 16 pages.") != thinking || !thinking && reasoning.Len() != 0 {
				t.Fatalf("incorrect native search delivery: %+v body=%s", result, rec.Body.String())
			}
		})
	}
}
