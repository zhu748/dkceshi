package responses

import (
	diagnosticapi "ds2api/internal/httpapi/tooldiagnostics"
	"ds2api/internal/promptcompat"
	"ds2api/internal/tooldiagnostics"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestStreamStorageFailureDoesNotClaimDiagnosticDelivery(t *testing.T) {
	s := tooldiagnostics.New(filepath.Join(t.TempDir(), "diag.json"))
	handler := diagnosticapi.Middleware(s)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		runtime := newResponsesStreamRuntime(w, http.NewResponseController(w), true, "resp", "model", "prompt", false, false, true, nil, nil, true, true, promptcompat.DefaultToolChoicePolicy(), "", func(map[string]any) bool { return false }, nil)
		runtime.toolCallRepairCtx = r.Context()
		const raw = `<tool_calls><invoke name="write"></invoke></tool_calls>`
		runtime.accumulator.RawText.WriteString(raw)
		runtime.accumulator.Text.WriteString(raw)
		runtime.finalize("stop", false)
	}))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/responses", nil))
	body := w.Body.String()
	if !strings.Contains(body, responseStorageCode) || strings.Contains(body, "function_call_arguments") || strings.Contains(body, `"type":"function_call"`) {
		t.Fatalf("incorrect output: %s", body)
	}
	page := s.List(tooldiagnostics.Query{})
	if page.Total != 1 {
		t.Fatal("missing record")
	}
	record, err := s.Get(page.Items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != "failed" || !record.Pinned || record.ErrorCode != responseStorageCode || record.Attempts[0].Calls[0].Delivery != "not_delivered" {
		t.Fatalf("incorrect delivery: %+v", record)
	}
}
