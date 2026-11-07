package gemini

import (
	"context"
	diagnosticapi "ds2api/internal/httpapi/tooldiagnostics"
	"ds2api/internal/sse"
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
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel_after_terminal", true: "terminal_write_failure"}[fail], func(t *testing.T) {
			store := tooldiagnostics.New(filepath.Join(t.TempDir(), "diag.json"))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			writer := &receiptBoundaryWriter{ResponseRecorder: httptest.NewRecorder(), cancel: cancel}
			if fail {
				writer.failToken = "\"finishReason\":\"STOP\""
			} else {
				writer.cancelToken = "\"finishReason\":\"STOP\""
			}
			handler := diagnosticapi.Middleware(store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				const raw = `<tool_calls><invoke name="first"></invoke><invoke name="second"></invoke></tool_calls>`
				rt := newGeminiStreamRuntime(w, http.NewResponseController(w), true, "model", "prompt", false, false, true, nil, nil, false, nil)
				rt.toolCallRepairCtx = r.Context()
				rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: raw}}})
				rt.finalize(false)
			}))
			handler.ServeHTTP(writer, httptest.NewRequest("POST", "/v1beta/models/model:streamGenerateContent", nil).WithContext(ctx))
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
			for _, call := range a.Calls {
				if call.Delivery != "written" {
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
