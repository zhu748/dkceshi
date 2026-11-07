package completionruntime

import (
	"context"
	"path/filepath"
	"testing"

	"ds2api/internal/assistantturn"
	"ds2api/internal/promptcompat"
	"ds2api/internal/tooldiagnostics"
)

func TestCancellationDoesNotHideTerminalWriteFailure(t *testing.T) {
	s := tooldiagnostics.New(filepath.Join(t.TempDir(), "diag.json"))
	r := tooldiagnostics.NewRecorder(s, "chat", true)
	ctx, cancel := context.WithCancel(tooldiagnostics.WithRecorder(context.Background(), r))
	tooldiagnostics.Observe(ctx, tooldiagnostics.Observation{Relevant: true, Calls: []tooldiagnostics.Call{{Ordinal: 1, Parse: "success", Delivery: "not_delivered"}}})
	tooldiagnostics.ToolsWritten(ctx)
	cancel()
	recordStreamFailure(ctx, promptcompat.StandardRequest{}, "", StreamRetryHooks{EmptyOutputError: func() *assistantturn.OutputError {
		return &assistantturn.OutputError{Status: 499, Code: "client_write_error", Message: "write deadline failed"}
	}}, nil)
	r.Finish(200, nil, true)
	record, err := s.Get(s.List(tooldiagnostics.Query{}).Items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != "failed" || record.Delivery != "write_failed" || record.Error != "write deadline failed" || record.Attempts[0].Validation != "success" || record.Attempts[0].Calls[0].Delivery != "written" {
		t.Fatalf("root failure lost: %+v", record)
	}
}
