package completionruntime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"ds2api/internal/tooldiagnostics"
)

func TestRequestSpecificErrorsDoNotPauseOtherRepairInputs(t *testing.T) {
	for _, status := range []int{400, 422} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					w.WriteHeader(status)
					return
				}
				if err := json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": "repaired"}}}}); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			settings := tooldiagnostics.RepairSettings{Mode: "custom", BaseURL: server.URL, Model: "model"}
			if _, err := callRepairAPI(context.Background(), settings, "oversized input"); err == nil {
				t.Fatal("expected first request failure")
			}
			output, err := callRepairAPI(context.Background(), settings, "smaller input")
			if err != nil || output.Text != "repaired" || calls.Load() != 2 {
				t.Fatalf("unrelated input blocked: %+v %v calls=%d", output, err, calls.Load())
			}
		})
	}
}

func TestSuccessfulProbeReopensPausedConfiguration(t *testing.T) {
	var failed atomic.Bool
	failed.Store(true)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if failed.Load() {
			w.WriteHeader(401)
			return
		}
		text := `<tool_calls><invoke name="repair_probe"><parameter name="path"><![CDATA[DS_SLOT_0]]></parameter></invoke></tool_calls>`
		if err := json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": text}}}}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	settings := tooldiagnostics.RepairSettings{Mode: "custom", BaseURL: server.URL, Model: "model", TimeoutSeconds: 5}
	if _, err := callRepairAPI(context.Background(), settings, "first"); err == nil {
		t.Fatal("expected auth failure")
	}
	failed.Store(false)
	if result, err := TestRepairAPI(context.Background(), settings); err != nil || !result.Success {
		t.Fatalf("probe failed: %+v %v", result, err)
	}
	if _, err := callRepairAPI(context.Background(), settings, "next"); err != nil || calls.Load() != 3 {
		t.Fatalf("successful probe did not reopen API: %v calls=%d", err, calls.Load())
	}
}
