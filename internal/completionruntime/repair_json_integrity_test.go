package completionruntime

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"ds2api/internal/tooldiagnostics"
)

func TestRepairTransportRejectsAmbiguousJSONForEveryProtocol(t *testing.T) {
	for _, protocol := range []string{"openai", "claude", "gemini"} {
		for _, raw := range []string{`{"content":"first","content":"second"}`, `{"content":"\ud800"}`, "{\"content\":\"\xff\"}"} {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if _, err := io.WriteString(w, raw); err != nil {
					t.Error(err)
				}
			}))
			_, err := doRepairAPIRequest(context.Background(), tooldiagnostics.RepairSettings{Protocol: protocol}, http.MethodGet, server.URL, nil, 1024)
			server.Close()
			if err == nil {
				t.Errorf("%s repair accepted ambiguous JSON %s", protocol, raw)
			}
		}
	}
}
