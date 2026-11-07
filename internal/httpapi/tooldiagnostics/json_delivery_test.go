package tooldiagnostics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"ds2api/internal/assistantturn"
	"ds2api/internal/sse"
	"ds2api/internal/tooldiagnostics"
	"ds2api/internal/util"
)

type cancelJSONWriter struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
}

func (w *cancelJSONWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseRecorder.Write(data)
	w.cancel()
	return n, err
}

func TestJSONDeliverySurvivesLateCancellationAndRejectsEncodingFailure(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		s := tooldiagnostics.New(filepath.Join(t.TempDir(), "diag.json"))
		ctx, cancel := context.WithCancel(context.Background())
		w := &cancelJSONWriter{httptest.NewRecorder(), cancel}
		h := Middleware(s)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assistantturn.BuildTurnFromCollected(sse.CollectResult{Text: `<tool_calls><invoke name="read"></invoke></tool_calls>`}, assistantturn.BuildOptions{DiagnosticContext: r.Context()})
			var payload any = map[string]any{"tool_calls": []any{map[string]any{"name": "read"}}}
			if invalid {
				payload = make(chan int)
			}
			util.WriteJSON(w, 200, payload)
		}))
		h.ServeHTTP(w, httptest.NewRequest("POST", "/v1/chat/completions", nil).WithContext(ctx))
		cancel()
		record, err := s.Get(s.List(tooldiagnostics.Query{}).Items[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		if invalid {
			if record.Status != "failed" || record.Delivery != "write_failed" {
				t.Fatal("encoding failure reported successful")
			}
		} else if record.Status != "direct_success" || record.Pinned || record.Attempts[0].Calls[0].Delivery != "written" {
			t.Fatalf("lost JSON delivery: %+v", record)
		}
	}
}
