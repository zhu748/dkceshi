package completionruntime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/tooldiagnostics"
)

func TestRepairAPIProbeUsesGenerationAndIntegrityForEveryProtocol(t *testing.T) {
	for _, protocol := range []string{"openai", "claude", "gemini"} {
		for _, valid := range []bool{false, true} {
			t.Run(protocol+map[bool]string{true: "/valid", false: "/changed-value"}[valid], func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != "POST" {
						t.Error("probe only fetched models")
						w.WriteHeader(405)
						return
					}
					var payload map[string]any
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Error(err)
					}
					value := "DS_SLOT_0"
					if !valid {
						value = "changed.txt"
					}
					text := `<tool_calls><invoke name="repair_probe"><parameter name="path"><![CDATA[` + value + `]]></parameter></invoke></tool_calls>`
					var response any
					switch protocol {
					case "claude":
						response = map[string]any{"stop_reason": "end_turn", "content": []any{map[string]any{"type": "text", "text": text}}}
					case "gemini":
						response = map[string]any{"candidates": []any{map[string]any{"finishReason": "STOP", "content": map[string]any{"parts": []any{map[string]any{"text": text}}}}}}
					default:
						response = map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": text}}}}
					}
					if err := json.NewEncoder(w).Encode(response); err != nil {
						t.Error(err)
					}
				}))
				defer server.Close()
				result, err := TestRepairAPI(context.Background(), tooldiagnostics.RepairSettings{Mode: "custom", Protocol: protocol, BaseURL: server.URL, Model: "probe-model", TimeoutSeconds: 5})
				if (err == nil) != valid || result.Success != valid {
					t.Fatalf("invalid probe result: %+v %v", result, err)
				}
			})
		}
	}
}

func TestCustomRepairHasIndependentResponseBudget(t *testing.T) {
	t.Setenv("DS2API_STREAM_MAX_REPAIR_BYTES", "64")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]any{"padding": strings.Repeat("x", 512)}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	_, err := callRepairAPI(context.Background(), tooldiagnostics.RepairSettings{BaseURL: server.URL, Model: "model"}, "small prompt")
	if err == nil || !strings.Contains(err.Error(), "output limit") {
		t.Fatalf("oversized repair accepted: %v", err)
	}
}
