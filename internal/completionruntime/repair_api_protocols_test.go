package completionruntime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/toolcall"
	"ds2api/internal/tooldiagnostics"
)

func TestCustomRepairWireProtocolsPreserveFinalTextAndSeparateThinking(t *testing.T) {
	const source = `<|MISTYPED|broken><|MISTYPED|invoke name="read"><|MISTYPED|parameter name="path">a.txt</|MISTYPED|parameter></|MISTYPED|invoke></|MISTYPED|broken>`
	const answer = `<tool_calls><invoke name="read"><parameter name="path">a.txt</parameter></invoke></tool_calls>`
	for _, protocol := range []string{"openai", "claude", "gemini"} {
		t.Run(protocol, func(t *testing.T) {
			requests := make(chan map[string]any, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				requests <- body
				var response any
				switch protocol {
				case "openai":
					if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer wire-key" {
						t.Error("wrong OpenAI route/auth")
					}
					response = map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": answer, "reasoning_content": "separate thinking"}}}}
				case "claude":
					if r.URL.Path != "/v1/messages" || r.Header.Get("x-api-key") != "wire-key" || r.Header.Get("anthropic-version") == "" || r.Header.Get("Authorization") != "" {
						t.Error("wrong Claude route/auth")
					}
					response = map[string]any{"stop_reason": "end_turn", "content": []any{map[string]any{"type": "thinking", "thinking": "ignore this candidate"}, map[string]any{"type": "text", "text": answer}}}
				case "gemini":
					if r.URL.Path != "/v1beta/models/test-model:generateContent" || r.Header.Get("x-goog-api-key") != "wire-key" || r.URL.Query().Get("key") != "" || r.Header.Get("Authorization") != "" {
						t.Error("wrong Gemini route/auth")
					}
					response = map[string]any{"candidates": []any{map[string]any{"finishReason": "STOP", "content": map[string]any{"parts": []any{map[string]any{"text": "ignore this candidate", "thought": true}, map[string]any{"text": answer}}}}}}
				}
				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(response); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			settings := tooldiagnostics.RepairSettings{Mode: "custom", Protocol: protocol, BaseURL: server.URL, Model: "test-model", APIKey: "wire-key", TimeoutSeconds: 5}
			if protocol == "gemini" {
				settings.ExtraBody = json.RawMessage(`{"generationConfig":{"thinkingConfig":{"includeThoughts":true}}}`)
			}
			ctx := tooldiagnostics.WithRepairSettings(context.Background(), settings)
			ds := &fakeDeepSeekCaller{}
			history := newRepairHistoryStore(t)
			calls, ok := toolcall.RepairToolCallsWithLLM(ctx, source, NewToolCallRepairInvoker(ds, &auth.RequestAuth{}, history))
			if !ok || len(calls) != 1 || calls[0].Name != "read" || calls[0].Input["path"] != "a.txt" {
				t.Fatalf("protocol repair failed: %+v", calls)
			}
			body := <-requests
			if protocol == "gemini" {
				if body["messages"] != nil || len(body["contents"].([]any)) != 1 {
					t.Fatal("Gemini context was not isolated")
				}
				generation := body["generationConfig"].(map[string]any)
				if generation["maxOutputTokens"] != float64(8192) || generation["candidateCount"] != float64(1) || generation["thinkingConfig"] == nil {
					t.Fatal("Gemini config merge lost defaults or thinking")
				}
			} else {
				if body["contents"] != nil || body["model"] != "test-model" || len(body["messages"].([]any)) != 1 {
					t.Fatal("message API context/model invalid")
				}
				if protocol == "claude" && body["max_tokens"] != float64(8192) {
					t.Fatal("required Claude max_tokens missing")
				}
			}
			archive, err := history.Get(repairEntries(t, history)[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			if archive.Content != answer || archive.ReasoningContent == "" {
				t.Fatal("protocol response mixed thinking and final content")
			}
		})
	}
}

func TestNativeRepairProtocolsRejectIncompleteOrStructuredToolResponses(t *testing.T) {
	for _, tc := range []struct{ protocol, body string }{
		{"claude", `{"stop_reason":"max_tokens","content":[{"type":"text","text":"partial"}]}`},
		{"claude", `{"stop_reason":"tool_use","content":[{"type":"tool_use","name":"unexpected"}]}`},
		{"gemini", `{"promptFeedback":{"blockReason":"SAFETY"}}`},
		{"gemini", `{"candidates":[{"finishReason":"MAX_TOKENS","content":{"parts":[{"text":"partial"}]}}]}`},
		{"gemini", `{"candidates":[{"finishReason":"STOP","content":{"parts":[{"functionCall":{"name":"unexpected"}}]}}]}`},
	} {
		var err error
		if tc.protocol == "claude" {
			_, err = parseClaudeRepairOutput([]byte(tc.body))
		} else {
			_, err = parseGeminiRepairOutput([]byte(tc.body))
		}
		if err == nil {
			t.Fatalf("unsafe %s response accepted", tc.protocol)
		}
	}
}
