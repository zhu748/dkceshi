package completionruntime

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
)

type interruptedCompletionReader struct{}

func (interruptedCompletionReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestNonStreamRejectsInterruptedResponseWithPartialOutput(t *testing.T) {
	for _, output := range []string{
		"partial answer",
		`<tool_calls><invoke name="run"><parameter name="cmd">ls</parameter></invoke></tool_calls>`,
	} {
		payload := "data: {\"p\":\"response/content\",\"v\":\"" + strings.ReplaceAll(output, `"`, `\"`) + "\"}\n"
		ds := &fakeDeepSeekCaller{responses: []*http.Response{{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(io.MultiReader(strings.NewReader(payload), interruptedCompletionReader{})),
		}}}
		_, outErr := ExecuteNonStreamWithRetry(context.Background(), ds, &auth.RequestAuth{}, promptcompat.StandardRequest{
			ResponseModel: "deepseek-v4.1-flash", FinalPrompt: "hello", ToolNames: []string{"run"},
			ToolChoice: promptcompat.DefaultToolChoicePolicy(),
		}, Options{RetryEnabled: true})
		if outErr == nil || outErr.Status != http.StatusBadGateway || outErr.Code != "upstream_read_error" {
			t.Fatalf("interrupted response accepted: error=%+v", outErr)
		}
		if len(ds.payloads) != 1 {
			t.Fatalf("interrupted output must not enter empty-output retries: %d requests", len(ds.payloads))
		}
	}
}

func TestNonStreamRejectsCleanEOFWithoutCompletionMarker(t *testing.T) {
	ds := &fakeDeepSeekCaller{responses: []*http.Response{{StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader("data: {\"p\":\"response/content\",\"v\":\"partial\"}\n")),
	}}}
	result, outErr := ExecuteNonStreamWithRetry(context.Background(), ds, &auth.RequestAuth{}, promptcompat.StandardRequest{}, Options{})
	if outErr == nil || outErr.Code != "upstream_read_error" || result.Turn.Text != "partial" {
		t.Fatalf("truncated answer accepted: result=%+v error=%+v", result, outErr)
	}
}

func TestRepairRejectsInterruptedAndUpstreamErrorResponses(t *testing.T) {
	for _, body := range []io.Reader{
		io.MultiReader(strings.NewReader("data: {\"v\":\"partial repair\"}\n"), interruptedCompletionReader{}),
		strings.NewReader("data: {\"error\":\"repair failed\"}\n"),
	} {
		ds := &fakeDeepSeekCaller{responses: []*http.Response{{StatusCode: http.StatusOK, Body: io.NopCloser(body)}}}
		text, err := NewToolCallRepairInvoker(ds, &auth.RequestAuth{}, nil)(context.Background(), "repair")
		if err == nil || text != "" {
			t.Fatalf("failed repair treated as usable output: text=%q, error=%v", text, err)
		}
	}
}
