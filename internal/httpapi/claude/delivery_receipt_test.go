package claude

import (
	"context"
	diagnosticapi "ds2api/internal/httpapi/tooldiagnostics"
	"ds2api/internal/tooldiagnostics"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

type receiptBoundaryWriter struct {
	*httptest.ResponseRecorder
	cancel                 context.CancelFunc
	cancelToken, failToken string
}

func (w *receiptBoundaryWriter) Write(b []byte) (int, error) {
	if w.failToken != "" && strings.Contains(string(b), w.failToken) {
		return 0, errors.New("test broken socket")
	}
	n, err := w.ResponseRecorder.Write(b)
	if w.cancelToken != "" && strings.Contains(string(b), w.cancelToken) {
		w.cancel()
	}
	return n, err
}
func TestDiagnosticTracksToolAndTerminalWrites(t *testing.T) {
	for _, scenario := range []string{"cancel_after_terminal", "terminal_write_failure", "second_tool_write_failure"} {
		fail := scenario != "cancel_after_terminal"
		t.Run(scenario, func(t *testing.T) {
			store := tooldiagnostics.New(filepath.Join(t.TempDir(), "diag.json"))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			writer := &receiptBoundaryWriter{ResponseRecorder: httptest.NewRecorder(), cancel: cancel}
			if fail {
				writer.failToken = "event: message_stop"
			} else {
				writer.cancelToken = "event: message_stop"
			}
			if scenario == "second_tool_write_failure" {
				writer.failToken = `"name":"second"`
			}
			handler := diagnosticapi.Middleware(store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				const raw = `<tool_calls><invoke name="first"></invoke><invoke name="second"></invoke></tool_calls>`
				rt := newClaudeStreamRuntime(w, http.NewResponseController(w), true, "model", nil, false, false, true, nil, nil, "prompt", nil)
				rt.toolCallRepairCtx = r.Context()
				rt.rawText.WriteString(raw)
				rt.text.WriteString(raw)
				rt.finalize("end_turn", false)
			}))
			handler.ServeHTTP(writer, httptest.NewRequest("POST", "/v1/messages", nil).WithContext(ctx))
			page := store.List(tooldiagnostics.Query{})
			if page.Total != 1 {
				t.Fatal(page)
			}
			record, err := store.Get(page.Items[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			a := record.Attempts[0]
			if a.Validation != "success" || len(a.Calls) != 2 {
				t.Fatalf("lost validation: %+v", a)
			}
			for i, call := range a.Calls {
				want := "written"
				if scenario == "second_tool_write_failure" && i == 1 {
					want = "not_delivered"
				}
				if call.Delivery != want {
					t.Fatalf("lost written tool: %+v", record)
				}
			}
			if fail {
				if record.Status != "failed" || record.Delivery != "write_failed" || a.ResponseWrittenAt != 0 {
					t.Fatalf("hidden failure: %+v", record)
				}
			} else if record.Status != "direct_success" || record.Pinned || a.ResponseWrittenAt == 0 {
				t.Fatalf("late cancellation overwrote success: %+v", record)
			}
		})
	}
}
