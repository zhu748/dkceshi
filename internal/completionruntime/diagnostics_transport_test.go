package completionruntime

import (
	"context"
	"ds2api/internal/assistantturn"
	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
	"ds2api/internal/tooldiagnostics"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

func TestInterruptedNonStreamKeepsDiagnosticWithoutRepair(t *testing.T) {
	for _, suffix := range []io.Reader{strings.NewReader(""), interruptedCompletionReader{}} {
		s := tooldiagnostics.New(filepath.Join(t.TempDir(), "diag.json"))
		r := tooldiagnostics.NewRecorder(s, "chat", false)
		ctx := tooldiagnostics.WithRecorder(context.Background(), r)
		const data = "data: {\"p\":\"response/content\",\"v\":\"<tool_calls><invoke name=\\\"run\\\"></invoke></tool_calls>\"}\n\n"
		ds := &fakeDeepSeekCaller{responses: []*http.Response{{StatusCode: 200, Body: io.NopCloser(io.MultiReader(strings.NewReader(data), suffix))}}}
		result, outErr := ExecuteNonStreamWithRetry(ctx, ds, &auth.RequestAuth{}, promptcompat.StandardRequest{ResponseModel: "test", ToolNames: []string{"run"}}, Options{ToolCallRepairEnabled: true, RetryEnabled: true})
		if outErr == nil || outErr.Code != "upstream_read_error" || len(result.Turn.ToolCalls) != 1 {
			t.Fatalf("unexpected result: %+v %+v", outErr, result.Turn)
		}
		r.Finish(502, nil, false)
		page := s.List(tooldiagnostics.Query{})
		if page.Total != 1 || len(ds.payloads) != 1 {
			t.Fatalf("lost evidence or retried: %+v", page)
		}
		record, err := s.Get(page.Items[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		if !record.Pinned || record.ErrorCode != "upstream_read_error" || record.Attempts[0].Raw.Text != result.Turn.RawText || record.Attempts[0].Calls[0].Delivery != "not_delivered" {
			t.Fatalf("bad evidence: %+v", record)
		}
	}
}
func TestCancelledStreamsKeepOnlyToolRelevantOutput(t *testing.T) {
	for _, raw := range []string{"ordinary prose", `<tool_calls><invoke name="read"></invoke></tool_calls>`} {
		s := tooldiagnostics.New(filepath.Join(t.TempDir(), "diag.json"))
		r := tooldiagnostics.NewRecorder(s, "stream", true)
		ctx, cancel := context.WithCancel(tooldiagnostics.WithRecorder(context.Background(), r))
		ExecuteStreamWithRetry(ctx, nil, &auth.RequestAuth{}, sseHTTPResponse(200, "data: [DONE]"), nil, "", StreamRetryOptions{Request: promptcompat.StandardRequest{ResponseModel: "model"}}, StreamRetryHooks{DiagnosticOutput: func() (string, string, string) { return raw, "", "" }, ConsumeAttempt: func(resp *http.Response, _ bool) ConsumeAttemptResult {
			if err := resp.Body.Close(); err != nil {
				t.Error(err)
			}
			cancel()
			return ConsumeAttemptResult{Terminal: true}
		}})
		r.Finish(200, nil, true)
		page := s.List(tooldiagnostics.Query{})
		if raw == "ordinary prose" {
			if page.Total != 0 {
				t.Fatal("ordinary request created tool diagnostic")
			}
			continue
		}
		if page.Total != 1 || page.Items[0].Status != "interrupted" {
			t.Fatalf("lost cancelled tools: %+v", page)
		}
	}
}
func TestTerminalFailureKeepsValidatedEvidence(t *testing.T) {
	s := tooldiagnostics.New(filepath.Join(t.TempDir(), "diag.json"))
	r := tooldiagnostics.NewRecorder(s, "stream", true)
	ctx := tooldiagnostics.WithRecorder(context.Background(), r)
	tooldiagnostics.Observe(ctx, tooldiagnostics.Observation{Relevant: true, Raw: "validated", Calls: []tooldiagnostics.Call{{Name: "read", Delivery: "not_delivered"}}})
	recordStreamFailure(ctx, promptcompat.StandardRequest{}, "", StreamRetryHooks{DiagnosticOutput: func() (string, string, string) { return "transport text", "", "" }}, &assistantturn.OutputError{Code: "response_storage_failed", Message: "full"})
	r.Finish(200, nil, false)
	record, err := s.Get(s.List(tooldiagnostics.Query{}).Items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != "failed" || record.Attempts[0].Raw.Text != "validated" || len(record.Attempts[0].Calls) != 1 {
		t.Fatalf("evidence overwritten: %+v", record)
	}
}
func TestRepairOutputRejectsStructuredCallsAndRefusal(t *testing.T) {
	for _, field := range []string{`"tool_calls":[{"type":"function","function":{"name":"unexpected"}}]`, `"function_call":{"name":"unexpected"}`, `"refusal":"cannot repair"`} {
		body := `{"choices":[{"finish_reason":"stop","message":{"content":"repaired",` + field + `}}]}`
		if _, err := parseOpenAIRepairOutput([]byte(body), ""); err == nil {
			t.Fatal("non-text repair accepted")
		}
	}
	body := `{"choices":[{"finish_reason":"stop","message":{"content":"repaired","tool_calls":[],"function_call":null,"refusal":null}}]}`
	out, err := parseOpenAIRepairOutput([]byte(body), "")
	if err != nil || out.Text != "repaired" {
		t.Fatalf("empty optional fields rejected: %+v %v", out, err)
	}
}
