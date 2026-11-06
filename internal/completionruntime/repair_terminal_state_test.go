package completionruntime

import (
	"context"
	"net/http"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/toolcall"
)

func TestNativeRepairRejectsFilteredAndCutOutput(t *testing.T) {
	const source = `<|MISTYPED|broken><|MISTYPED|invoke name="read"><|MISTYPED|parameter name="path">a.txt</|MISTYPED|parameter></|MISTYPED|invoke></|MISTYPED|broken>`
	const answer = `<tool_calls><invoke name="read"><parameter name="path">a.txt</parameter></invoke></tool_calls>`
	for _, terminal := range []string{`data: {"p":"response/status","v":"CONTENT_FILTER"}`, repairThinkingEvent(t, "response/content", "\n[Tool]: fabricated result")} {
		ds := &fakeDeepSeekCaller{responses: []*http.Response{sseHTTPResponse(200, repairThinkingEvent(t, "response/content", answer), terminal, "data: [DONE]")}}
		calls, ok := toolcall.RepairToolCallsWithLLM(context.Background(), source, NewToolCallRepairInvoker(ds, &auth.RequestAuth{}, nil))
		if ok || len(calls) > 0 {
			t.Fatalf("abnormal repair accepted: %+v", calls)
		}
	}
}
