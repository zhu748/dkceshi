package chat

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
	"ds2api/internal/tooldiagnostics"
)

func TestVercelProgressIsAuthenticatedBoundedAndNonExecutable(t *testing.T) {
	t.Setenv("VERCEL", "1")
	t.Setenv("DS2API_VERCEL_INTERNAL_SECRET", "internal-test")
	s := tooldiagnostics.New(filepath.Join(t.TempDir(), "diag.json"))
	h := &Handler{ToolDiagnostics: s}
	id := h.holdStreamLease(&auth.RequestAuth{}, promptcompat.StandardRequest{ResponseModel: "model"}, "session")
	h.attachLeaseDiagnostics(id, promptcompat.StandardRequest{ResponseModel: "model"})
	body, err := json.Marshal(map[string]any{"lease_id": id, "diagnostic_attempt": 1, "content": tooldiagnostics.Capture(`<tool_calls><invoke name="read">`)})
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"wrong", "internal-test"} {
		r := httptest.NewRequest("POST", "/v1/chat/completions?__stream_progress=1", strings.NewReader(string(body)))
		r.Header.Set("X-Ds2-Internal-Token", secret)
		w := httptest.NewRecorder()
		h.ChatCompletions(w, r)
		if secret == "wrong" {
			if w.Code != 401 || s.List(tooldiagnostics.Query{}).Total != 0 {
				t.Fatal("unauthorized progress recorded")
			}
			continue
		}
		if w.Code != 200 {
			t.Fatalf("progress failed: %s", w.Body.String())
		}
	}
	page := s.List(tooldiagnostics.Query{})
	if page.Total != 1 {
		t.Fatal("candidate not saved")
	}
	record, err := s.Get(page.Items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.CompletedAt != 0 || len(record.Attempts[0].Calls) != 0 || record.Delivery != "pending" {
		t.Fatal("progress finalized a tool")
	}
}
