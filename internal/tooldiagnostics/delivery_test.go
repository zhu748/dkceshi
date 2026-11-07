package tooldiagnostics

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDeliveryEvidencePreservesValidationAndPartialToolWrites(t *testing.T) {
	for _, tc := range []struct {
		name                          string
		complete, stopped, writeError bool
		status, delivery, second      string
	}{
		{"completed then cancelled", true, true, false, "direct_success", "written", "written"},
		{"one tool then cancelled", false, true, false, "interrupted", "unknown", "unknown"},
		{"one tool then write failed", false, true, true, "failed", "write_failed", "not_delivered"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := New(filepath.Join(t.TempDir(), "diag.json"))
			r := NewRecorder(s, "chat", true)
			ctx := WithRecorder(context.Background(), r)
			Configure(ctx, "alias", "caller", "account", true, nil, nil, "actual-model")
			Observe(ctx, Observation{Relevant: true, Raw: "validated source", Calls: []Call{
				{Ordinal: 1, Name: "read", Parse: "success", Schema: "passed", Delivery: "not_delivered"},
				{Ordinal: 2, Name: "read", Parse: "success", Schema: "passed", Delivery: "not_delivered"},
			}})
			ToolsWritten(ctx, 1)
			if tc.complete {
				ToolsWritten(ctx, 2)
				ResponseWritten(ctx)
				Failure(ctx, "request_cancelled", "late cancellation")
				Observe(ctx, Observation{Relevant: true, ErrorCode: "request_cancelled"})
			}
			var writeErr error
			if tc.writeError {
				writeErr = errors.New("broken pipe")
			}
			r.Finish(200, writeErr, tc.stopped)
			record, err := s.Get(s.List(Query{}).Items[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			a := record.Attempts[0]
			if record.Status != tc.status || record.Delivery != tc.delivery || a.Calls[0].Delivery != "written" || a.Calls[1].Delivery != tc.second || a.Validation != "success" || a.Model != "actual-model" {
				t.Fatalf("incorrect evidence: %+v", record)
			}
			if tc.complete && (record.Pinned || record.ErrorCode != "") {
				t.Fatal("late cancellation pinned successful tools")
			}
		})
	}
}

func TestDeliveryCallbacksCannotAffectDifferentAttempt(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "diag.json"))
	r := NewRecorder(s, "chat", true)
	ctx := WithRecorder(context.Background(), r)
	old := WithAttempt(ctx, 1)
	Observe(old, Observation{Relevant: true, ErrorCode: "invalid_tool_arguments"})
	NextAttempt(ctx, "regeneration", "invalid_tool_arguments", "retry", "account")
	Observe(ctx, Observation{Relevant: true, Calls: []Call{{Ordinal: 1, Parse: "success", Delivery: "not_delivered"}}})
	ToolsWritten(old)
	ResponseWritten(old)
	if r.current().ResponseWrittenAt != 0 || r.current().Calls[0].Delivery != "not_delivered" {
		t.Fatal("stale callback changed current attempt")
	}
	r.Finish(499, nil, true)
}

func TestUnreadableRepairConfigurationSuspendsModelCalls(t *testing.T) {
	path := filepath.Join(t.TempDir(), "diag.json")
	if err := os.WriteFile(path, []byte(`{"settings":`), 0600); err != nil {
		t.Fatal(err)
	}
	s := New(path)
	if s.RepairSettings().UnavailableReason == "" {
		t.Fatal("broken settings silently selected original model")
	}
}

func TestRestartKeepsCommittedTerminalReceipt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "diag.json")
	s := New(path)
	r := NewRecorder(s, "chat", true)
	ctx := WithRecorder(context.Background(), r)
	Observe(ctx, Observation{Relevant: true, Calls: []Call{{Ordinal: 1, Parse: "success", Delivery: "not_delivered"}}})
	ToolsWritten(ctx)
	ResponseWritten(ctx)
	reloaded := New(path)
	page := reloaded.List(Query{})
	if page.Total != 1 || page.Items[0].Status != "direct_success" || page.Items[0].Pinned {
		t.Fatalf("durable success lost: %+v", page)
	}
}
