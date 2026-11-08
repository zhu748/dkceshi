package completionruntime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ds2api/internal/tooldiagnostics"
)

func TestGeminiRepairRejectsTextWithAnySecondNativePayload(t *testing.T) {
	for _, key := range []string{"functionCall", "function_call", "functionResponse", "function_response", "inlineData", "inline_data", "fileData", "file_data", "executableCode", "codeExecutionResult", "attachments", "files"} {
		for _, value := range []any{nil, map[string]any{}} {
			part := map[string]any{"text": "REPAIR_TEXT", key: value}
			data, _ := json.Marshal(map[string]any{"candidates": []any{map[string]any{"finishReason": "STOP", "content": map[string]any{"parts": []any{part}}}}})
			if _, err := parseGeminiRepairOutput(data); err == nil {
				t.Fatalf("mixed repair response accepted %s", key)
			}
		}
	}
	data := []byte(`{"candidates":[{"finishReason":"STOP","content":{"parts":[{"text":"  reasoning\r\n","thought":true,"thoughtSignature":"opaque"},{"text":"  final\r\n","thought":false,"annotations":["metadata"]}]}}]}`)
	output, err := parseGeminiRepairOutput(data)
	if err != nil || output.Text != "  final\r\n" || output.Thinking != "  reasoning\r\n" {
		t.Fatalf("valid text/metadata was changed: %v %+v", err, output)
	}
}

func TestCustomRepairTransportRejectsMixedGeminiParts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"candidates":[{"finishReason":"STOP","content":{"parts":[{"text":"REPAIR_TEXT","functionCall":{"name":"unexpected","args":{}}}]}}]}`)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	_, err := callRepairAPI(context.Background(), tooldiagnostics.RepairSettings{Mode: "custom", Protocol: "gemini", Model: "audit-model", BaseURL: server.URL}, "repair only")
	if err == nil {
		t.Fatal("HTTP repair accepted a mixed executable payload")
	}
}
