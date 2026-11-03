package completionruntime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"ds2api/internal/tooldiagnostics"
)

func TestRepairRetriesTransportAndInvalidFormatWithOneBudget(t *testing.T) {
	for _, tc := range []struct {
		name       string
		retries    *int
		failStatus int
		failCount  int
		want       int
		success    bool
	}{
		{"default three retries", nil, 503, 3, 4, true},
		{"disabled", intPointer(0), 503, 3, 1, false},
		{"authentication is permanent", nil, 401, 3, 1, false},
		{"invalid repair format", intPointer(1), 200, 1, 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := int(calls.Add(1))
				text := `<tool_calls><invoke name="repair_probe"><parameter name="path"><![CDATA[DS_SLOT_0]]></parameter></invoke></tool_calls>`
				if n <= tc.failCount {
					if tc.failStatus != 200 {
						w.WriteHeader(tc.failStatus)
						if _, err := w.Write([]byte(`{"error":{"message":"unavailable"}}`)); err != nil {
							t.Error(err)
						}
						return
					}
					text = "NO TOOL"
				}
				if err := json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": text}}}}); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			result, err := TestRepairAPI(context.Background(), tooldiagnostics.RepairSettings{Mode: "custom", BaseURL: server.URL, Model: "model", RetryCount: tc.retries, TimeoutSeconds: 5})
			if result.Success != tc.success || (err == nil) != tc.success || int(calls.Load()) != tc.want || result.Attempts != tc.want {
				t.Fatalf("result=%+v err=%v calls=%d", result, err, calls.Load())
			}
		})
	}
}

func intPointer(value int) *int { return &value }

func TestPermanentConfigurationBackoffIsIsolatedAndProbeBypassesIt(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(401) }))
	defer server.Close()
	settings := tooldiagnostics.RepairSettings{Mode: "custom", BaseURL: server.URL, Model: "model", TimeoutSeconds: 2}
	for i := 0; i < 2; i++ {
		if _, err := callRepairAPI(context.Background(), settings, "prompt"); err == nil {
			t.Fatal("authentication error accepted")
		}
	}
	if calls.Load() != 1 {
		t.Fatal("repeated configuration failure was not paused")
	}
	settings.Model = "changed-model"
	if _, err := callRepairAPI(context.Background(), settings, "prompt"); err == nil {
		t.Fatal("authentication error accepted")
	}
	if calls.Load() != 2 {
		t.Fatal("changed config inherited pause")
	}
	if _, err := TestRepairAPI(context.Background(), settings); err == nil {
		t.Fatal("probe unexpectedly passed")
	}
	if calls.Load() != 3 {
		t.Fatal("explicit probe did not bypass pause")
	}
}

func TestRepairRetryAfterAndCancellationShareDeadline(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(429)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := TestRepairAPI(ctx, tooldiagnostics.RepairSettings{Mode: "custom", BaseURL: server.URL, Model: "model", TimeoutSeconds: 5})
	if err == nil || calls.Load() != 1 || time.Since(started) > time.Second {
		t.Fatalf("cancellation failed: calls=%d err=%v", calls.Load(), err)
	}
}

func TestPanelThinkingControlsUseNativeProtocolShapes(t *testing.T) {
	for _, tc := range []struct{ protocol, model, mode string }{
		{"openai", "reasoner", ""}, {"claude", "claude-sonnet-4-6", ""}, {"claude", "gateway-model", "budget"}, {"gemini", "gemini-3-flash", ""}, {"gemini", "gemini-2.5-pro", ""},
	} {
		t.Run(tc.protocol+tc.model, func(t *testing.T) {
			settings := tooldiagnostics.RepairSettings{Protocol: tc.protocol, Model: tc.model, ReasoningEffort: "high", ThinkingMode: tc.mode}
			body, err := buildRepairAPIPayload(settings, "prompt")
			if err != nil {
				t.Fatal(err)
			}
			switch tc.protocol {
			case "openai":
				if body["reasoning_effort"] != "high" {
					t.Fatal(body)
				}
			case "claude":
				thinking := repairObject(body["thinking"])
				if tc.mode == "budget" {
					if thinking["budget_tokens"] != 8192 || body["max_tokens"].(int) <= 8192 {
						t.Fatal(body)
					}
				} else if thinking["type"] != "adaptive" || repairObject(body["output_config"])["effort"] != "high" {
					t.Fatal(body)
				}
			case "gemini":
				generation := repairObject(body["generationConfig"])
				thinking := repairObject(generation["thinkingConfig"])
				if tc.model == "gemini-2.5-pro" {
					if thinking["thinkingBudget"] != 8192 || generation["maxOutputTokens"].(int) <= 8192 {
						t.Fatal(body)
					}
				} else if thinking["thinkingLevel"] != "high" {
					t.Fatal(body)
				}
			}
		})
	}
	// A panel choice must not erase unrelated advanced generation settings.
	settings := tooldiagnostics.RepairSettings{Protocol: "gemini", Model: "model", ReasoningEffort: "low", ExtraBody: json.RawMessage(`{"generation_config":{"temperature":0.2,"thinking_config":{"includeThoughts":true,"thinkingBudget":9999}}}`)}
	body, err := buildRepairAPIPayload(settings, "prompt")
	if err != nil {
		t.Fatal(err)
	}
	config := repairObject(body["generationConfig"])
	thinking := repairObject(config["thinkingConfig"])
	if config["temperature"] == nil || thinking["includeThoughts"] != true || thinking["thinkingLevel"] != "low" || thinking["thinkingBudget"] != nil {
		t.Fatal(body)
	}
}
