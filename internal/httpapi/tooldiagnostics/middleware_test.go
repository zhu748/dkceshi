package tooldiagnostics

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"ds2api/internal/assistantturn"
	"ds2api/internal/sse"
	"ds2api/internal/tooldiagnostics"
)

type brokenWriter struct{ header http.Header }

func (w *brokenWriter) Header() http.Header       { return w.header }
func (w *brokenWriter) WriteHeader(int)           {}
func (w *brokenWriter) Write([]byte) (int, error) { return 0, errors.New("disconnected") }

func TestMiddlewareRecordsActualWriteFailureAndSkipsInternalEndpoints(t *testing.T) {
	for _, tc := range []struct {
		path   string
		broken bool
		want   int
	}{
		{"/v1/chat/completions", false, 1}, {"/v1/chat/completions", true, 1}, {"/v1/chat/completions?__stream_prepare=1", false, 0}, {"/admin/test", false, 0},
	} {
		t.Run(tc.path, func(t *testing.T) {
			s := tooldiagnostics.New(filepath.Join(t.TempDir(), "diag.json"))
			handler := Middleware(s)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assistantturn.BuildTurnFromCollected(sse.CollectResult{Text: `<tool_calls><invoke name="read"></invoke></tool_calls>`}, assistantturn.BuildOptions{DiagnosticContext: r.Context()})
				_, err := w.Write([]byte("response"))
				if err != nil && !tc.broken {
					t.Fatal(err)
				}
			}))
			var writer http.ResponseWriter = httptest.NewRecorder()
			if tc.broken {
				writer = &brokenWriter{header: http.Header{}}
			}
			handler.ServeHTTP(writer, httptest.NewRequest(http.MethodPost, tc.path, nil))
			list := s.List(tooldiagnostics.Query{})
			if list.Total != tc.want {
				t.Fatalf("records=%d want=%d", list.Total, tc.want)
			}
			if tc.want > 0 {
				record, _ := s.Get(list.Items[0].ID)
				if tc.broken && (record.Delivery != "write_failed" || !record.Pinned) {
					t.Fatalf("write failure reported successful: %+v", record)
				}
				if !tc.broken && record.Delivery != "written" {
					t.Fatal("successful response not finalized")
				}
			}
		})
	}
}

func TestCustomRepairSettingsApplyWithCaptureDisabled(t *testing.T) {
	s := tooldiagnostics.New(filepath.Join(t.TempDir(), "diag.json"))
	if err := s.SetRepairSettings(tooldiagnostics.RepairSettings{Mode: "custom", BaseURL: "https://example.test/v1", Model: "repair", APIKey: "key", TimeoutSeconds: 60}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSettings(tooldiagnostics.Settings{Enabled: false, Limit: 20}); err != nil {
		t.Fatal(err)
	}
	handler := Middleware(s)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg := tooldiagnostics.RepairSettingsFromContext(r.Context())
		if cfg.Mode != "custom" || cfg.Model != "repair" || cfg.APIKey != "key" {
			t.Fatal("repair configuration depended on capture being enabled")
		}
		if tooldiagnostics.FromContext(r.Context()) != nil {
			t.Fatal("disabled capture created a recorder")
		}
		w.WriteHeader(204)
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
}
