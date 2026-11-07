package chat

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
	"ds2api/internal/tooldiagnostics"
)

func TestUsagePreviewDoesNotReleaseLeaseOrCompleteDiagnostics(t *testing.T) {
	t.Setenv("VERCEL", "1")
	t.Setenv("DS2API_VERCEL_INTERNAL_SECRET", "preview-secret")
	s := tooldiagnostics.New(filepath.Join(t.TempDir(), "diag.json"))
	h := &Handler{ToolDiagnostics: s}
	std := promptcompat.StandardRequest{ResponseModel: "model", FinalPrompt: "prompt"}
	owner := &auth.RequestAuth{}
	cleanups := 0
	owner.SetAccountFinalizer(func(bool) { cleanups++ })
	id := h.holdStreamLease(owner, std, "session")
	h.attachLeaseDiagnostics(id, std)
	lease, _ := h.lookupStreamLease(id)
	ctx := tooldiagnostics.WithRecorder(context.Background(), lease.Diagnostics)
	tooldiagnostics.Observe(ctx, tooldiagnostics.Observation{Relevant: true, Calls: []tooldiagnostics.Call{{Ordinal: 1, Name: "read", Parse: "success", Delivery: "not_delivered"}}})
	post := func(extra string) {
		r := httptest.NewRequest("POST", "/v1/chat/completions?__stream_release=1", strings.NewReader(`{"lease_id":"`+id+`","usage_accounting_version":1,"content":"answer",`+extra+`}`))
		r.Header.Set("X-Ds2-Internal-Token", "preview-secret")
		w := httptest.NewRecorder()
		h.handleVercelStreamRelease(w, r)
		if w.Code != 200 {
			t.Fatalf("release: %d %s", w.Code, w.Body.String())
		}
	}
	post(`"usage_only":true`)
	if cleanups != 0 {
		t.Fatal("usage preview ran cleanup")
	}
	if _, ok := h.lookupStreamLease(id); !ok {
		t.Fatal("usage preview released lease")
	}
	page := s.List(tooldiagnostics.Query{})
	if page.Items[0].CompletedAt != 0 {
		t.Fatal("usage preview completed delivery")
	}
	post(`"delivery_version":1,"tools_written":true,"response_written":true,"finish_reason":"tool_calls"`)
	if _, ok := h.lookupStreamLease(id); ok {
		t.Fatal("final release retained lease")
	}
	record, err := s.Get(page.Items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if cleanups != 1 {
		t.Fatal("final release did not run cleanup once")
	}
	if record.Status != "direct_success" || record.Delivery != "written" {
		t.Fatalf("missing final delivery evidence: %+v", record)
	}
}
