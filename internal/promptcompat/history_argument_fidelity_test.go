package promptcompat

import (
	"strings"
	"testing"
)

func TestResponsesNonObjectHistorySurvivesLiveAndFileTransport(t *testing.T) {
	for _, arguments := range []any{nil, "null", "  \r\n", `[]`, `{"broken":`} {
		req := map[string]any{"model": "deepseek-v4.1-flash", "input": []any{
			map[string]any{"type": "function_call", "name": "read", "call_id": "SOURCE_ID", "arguments": arguments},
			map[string]any{"type": "function_call_output", "call_id": "SOURCE_ID", "output": "ERROR_SENTINEL"},
		}}
		standard, err := NormalizeOpenAIResponsesRequest(nil, req, "")
		if err != nil {
			t.Fatal(err)
		}
		for _, text := range []string{standard.FinalPrompt, BuildOpenAIHistoryTranscript(standard.Messages)} {
			if arguments != `{"broken":` && !strings.Contains(text, "[Historical tool call with non-object arguments]") || !strings.Contains(text, "SOURCE_ID") || !strings.Contains(text, "ERROR_SENTINEL") {
				t.Fatalf("non-object arguments disappeared: %s", text)
			}
		}
	}
}
