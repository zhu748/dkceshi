package chat

import (
	"context"
	"ds2api/internal/auth"
	diagnosticapi "ds2api/internal/httpapi/tooldiagnostics"
	"ds2api/internal/promptcompat"
	"ds2api/internal/tooldiagnostics"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

type cancelOnOutputWriter struct {
	*httptest.ResponseRecorder
	trigger string
	cancel  context.CancelFunc
}

func (w *cancelOnOutputWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseRecorder.Write(b)
	if err == nil && w.trigger != "" && strings.Contains(string(b), w.trigger) {
		w.cancel()
	}
	return n, err
}

func TestSuccessfulStreamSurvivesCancellationAfterToolOrTerminalWrite(t *testing.T) {
	for _, trigger := range []string{"", `"tool_calls":[`, "data: [DONE]"} {
		t.Run(trigger, func(t *testing.T) {
			store := tooldiagnostics.New(filepath.Join(t.TempDir(), "diag.json"))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			writer := &cancelOnOutputWriter{ResponseRecorder: httptest.NewRecorder(), trigger: trigger, cancel: cancel}
			raw := `<tool_calls><invoke name="read"><parameter name="path">a.txt</parameter></invoke></tool_calls>`
			data, err := json.Marshal(map[string]any{"p": "response/content", "v": raw})
			if err != nil {
				t.Fatal(err)
			}
			std := promptcompat.StandardRequest{ResponseModel: "model", Stream: true, ToolNames: []string{"read"}}
			handler := diagnosticapi.Middleware(store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				h := &Handler{DS: &inlineUploadDSStub{}}
				h.handleStreamWithRetry(w, r, &auth.RequestAuth{}, makeOpenAISSEHTTPResponse("data: "+string(data), "data: [DONE]"), nil, "", "id", nil, std, "model", "prompt", 0, false, false, std.ToolNames, nil, std.ToolChoice, nil)
			}))
			handler.ServeHTTP(writer, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx))
			if !strings.Contains(writer.Body.String(), `"tool_calls":[`) || !strings.Contains(writer.Body.String(), "data: [DONE]") {
				t.Fatal("missing tool or terminal frame")
			}
			page := store.List(tooldiagnostics.Query{})
			if page.Total != 1 {
				t.Fatalf("records=%d", page.Total)
			}
			record, err := store.Get(page.Items[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			want := "direct_success"
			if record.Status != want {
				t.Fatalf("status=%s want=%s", record.Status, want)
			}
			if record.Delivery != "written" || record.Pinned || record.ErrorCode != "" || record.Attempts[0].Validation != "success" || record.Attempts[0].ResponseWrittenAt == 0 || record.Attempts[0].Calls[0].Delivery != "written" {
				t.Fatalf("incorrect successful delivery: %+v", record)
			}
		})
	}
}
