package completionruntime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"ds2api/internal/tooldiagnostics"
)

func TestInvalidRepairSnapshotStopsBeforeNetworkWithoutRetry(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	for _, protocol := range []string{"openai", "claude", "gemini"} {
		settings := tooldiagnostics.RepairSettings{Mode: "custom", Protocol: protocol, BaseURL: server.URL, Model: "repair", ExtraBody: json.RawMessage(`{"generationConfig":{"maxOutputTokens":8192,"max_output_tokens":1}}`)}
		_, err := callRepairAPI(context.Background(), settings, "ISOLATED_PROMPT")
		var apiError *repairAPIError
		if !errors.As(err, &apiError) || apiError.status != http.StatusBadRequest || apiError.Retryable() || calls.Load() != 0 {
			t.Fatalf("invalid local config reached network/retry: protocol=%s error=%v calls=%d", protocol, err, calls.Load())
		}
	}
}
