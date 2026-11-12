package translatorcliproxy

import (
	"encoding/json"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUsageMetadataSurvivesProtocolTranslation(t *testing.T) {
	for _, target := range []sdktranslator.Format{sdktranslator.FormatClaude, sdktranslator.FormatGemini} {
		t.Run(string(target), func(t *testing.T) {
			raw := []byte(`{"id":"chat_test","model":"model","choices":[{"index":0,"message":{"role":"assistant","content":"answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":29,"total_tokens":40,"completion_tokens_details":{"reasoning_tokens":9},"ds2api":{"source":"upstream","upstream":{"messages":[{"accumulated_tokens":2551}]}}}}`)
			converted := FromOpenAINonStream(target, "model", []byte(`{}`), []byte(`{}`), raw)
			assertTranslatedUsage(t, converted, target)
			rec := httptest.NewRecorder()
			streamRequest := []byte(`{"model":"model","stream":true,"messages":[{"role":"user","content":"task"}]}`)
			writer := NewOpenAIStreamTranslatorWriter(rec, target, "model", streamRequest, streamRequest)
			for _, line := range []string{
				`data: {"object":"chat.completion.chunk","id":"chat_test","model":"model","choices":[{"index":0,"delta":{"role":"assistant","content":"answer"},"finish_reason":null}]}`,
				`data: {"object":"chat.completion.chunk","id":"chat_test","model":"model","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":29,"total_tokens":40,"completion_tokens_details":{"reasoning_tokens":9},"ds2api":{"source":"upstream","upstream":{"messages":[{"accumulated_tokens":2551}]}}}}`,
				`data: [DONE]`,
			} {
				if _, err := writer.Write([]byte(line + "\n\n")); err != nil {
					t.Fatal(err)
				}
			}
			found := false
			for _, line := range strings.Split(rec.Body.String(), "\n") {
				if strings.Contains(line, `"ds2api"`) {
					assertTranslatedUsage(t, []byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), target)
					found = true
				}
			}
			if !found {
				t.Fatalf("stream translation dropped upstream evidence: %s", rec.Body)
			}
		})
	}
}

func assertTranslatedUsage(t *testing.T, raw []byte, target sdktranslator.Format) {
	t.Helper()
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatal(err)
	}
	key := "usage"
	if target == sdktranslator.FormatGemini {
		key = "usageMetadata"
	}
	usage, _ := obj[key].(map[string]any)
	meta, _ := usage["ds2api"].(map[string]any)
	if meta["source"] != "upstream" || meta["upstream"] == nil {
		t.Fatalf("metadata lost: %s", raw)
	}
	if target == sdktranslator.FormatClaude {
		if usage["input_tokens"] != float64(11) || usage["output_tokens"] != float64(29) {
			t.Fatalf("counts changed: %s", raw)
		}
	} else if usage["promptTokenCount"] != float64(11) || usage["candidatesTokenCount"] != float64(20) || usage["thoughtsTokenCount"] != float64(9) || usage["totalTokenCount"] != float64(40) {
		t.Fatalf("Gemini reasoning was double counted: %s", raw)
	}
}
