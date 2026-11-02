package chat

import (
	"ds2api/internal/auth"
	diagnosticapi "ds2api/internal/httpapi/tooldiagnostics"
	"ds2api/internal/promptcompat"
	"ds2api/internal/tooldiagnostics"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestInterruptedStreamWithoutIDKeepsDiagnostics(t *testing.T) {
	s := tooldiagnostics.New(filepath.Join(t.TempDir(), "diag.json"))
	const raw = `<tool_calls><invoke name="read"></invoke></tool_calls>`
	const data = "data: {\"p\":\"response/content\",\"v\":\"<tool_calls><invoke name=\\\"read\\\"></invoke></tool_calls>\"}\n\n"
	handler := diagnosticapi.Middleware(s)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := &Handler{}
		resp := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(data))}
		std := promptcompat.StandardRequest{ResponseModel: "model", Stream: true, ToolNames: []string{"read"}}
		h.handleStreamWithRetry(w, r, &auth.RequestAuth{}, resp, nil, "", "id", nil, std, "model", "prompt", 0, false, false, std.ToolNames, nil, std.ToolChoice, nil)
	}))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
	if !strings.Contains(w.Body.String(), "upstream_interrupted") || strings.Contains(w.Body.String(), `"tool_calls":[`) {
		t.Fatalf("incorrect output: %s", w.Body.String())
	}
	page := s.List(tooldiagnostics.Query{})
	if page.Total != 1 {
		t.Fatalf("lost interrupted tool: %+v", page)
	}
	record, err := s.Get(page.Items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !record.Pinned || record.Status != "failed" || record.ErrorCode != "upstream_interrupted" || record.Attempts[0].Raw.Text != raw || record.Attempts[0].Calls[0].Delivery != "not_delivered" {
		t.Fatalf("incorrect evidence: %+v", record)
	}
}
