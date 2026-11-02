package chat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
	"ds2api/internal/tooldiagnostics"
)

func TestVercelCustomRepairWorksWithDiagnosticCaptureDisabled(t *testing.T) {
	t.Setenv("VERCEL", "1")
	t.Setenv("DS2API_VERCEL_INTERNAL_SECRET", "internal-test-secret")
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["model"] != "custom-repair-model" || r.Header.Get("Authorization") != "Bearer repair-key" {
			t.Error("lease lost custom repair settings")
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": `<tool_calls><invoke name="read"><parameter name="path">a.txt</parameter></invoke></tool_calls>`}, "finish_reason": "stop"}}}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	s := tooldiagnostics.New(filepath.Join(t.TempDir(), "diag.json"))
	if err := s.SetRepairSettings(tooldiagnostics.RepairSettings{Mode: "custom", BaseURL: server.URL, Model: "custom-repair-model", APIKey: "repair-key", TimeoutSeconds: 60}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSettings(tooldiagnostics.Settings{Enabled: false, Limit: 20}); err != nil {
		t.Fatal(err)
	}
	h := &Handler{ToolDiagnostics: s, DS: &inlineUploadDSStub{}}
	std := promptcompat.StandardRequest{ResolvedModel: "deepseek-v4.1-flash", ResponseModel: "compatible-alias", ToolNames: []string{"read"}}
	id := h.holdStreamLease(&auth.RequestAuth{}, std, "session")
	h.attachLeaseDiagnostics(id, std)
	if h.leaseRepairTimeoutMS(id) != 60000 {
		t.Fatal("configured timeout not propagated to Node prepare")
	}
	data, err := json.Marshal(map[string]any{"lease_id": id, "content": `<|MISTYPED|broken><|MISTYPED|invoke name="read"><|MISTYPED|parameter name="path">a.txt</|MISTYPED|parameter></|MISTYPED|invoke></|MISTYPED|broken>`})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?__stream_validate=1", strings.NewReader(string(data)))
	req.Header.Set("X-Ds2-Internal-Token", "internal-test-secret")
	w := httptest.NewRecorder()
	h.ChatCompletions(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"tool_calls"`) || requests.Load() != 1 {
		t.Fatalf("Vercel custom repair failed: %d %s", w.Code, w.Body.String())
	}
	if s.List(tooldiagnostics.Query{}).Total != 0 || strings.Contains(w.Body.String(), "repair-key") {
		t.Fatal("capture disabled or credential isolation was ignored")
	}
}
