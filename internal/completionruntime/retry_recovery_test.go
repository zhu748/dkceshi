package completionruntime

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"ds2api/internal/account"
	"ds2api/internal/assistantturn"
	"ds2api/internal/auth"
	"ds2api/internal/config"
	"ds2api/internal/promptcompat"
)

func TestNonStreamRegeneratesInvalidToolArguments(t *testing.T) {
	tools := []any{map[string]any{"type": "function", "function": map[string]any{"name": "lookup", "parameters": map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}, "required": []any{"query"}}}}}
	bad := `data: {"p":"response/content","v":"<tool_calls><invoke name=\"lookup\"></invoke></tool_calls>"}`
	good := `data: {"p":"response/content","v":"<tool_calls><invoke name=\"lookup\"><parameter name=\"query\">ok</parameter></invoke></tool_calls>"}`
	ds := &fakeDeepSeekCaller{responses: []*http.Response{sseHTTPResponse(http.StatusOK, good)}}
	std := promptcompat.StandardRequest{FinalPrompt: "original", PromptTokenText: "original", ToolNames: []string{"lookup"}, ToolsRaw: tools}
	result, outErr := ExecuteNonStreamStartedWithRetry(context.Background(), ds, &auth.RequestAuth{}, StartResult{Response: sseHTTPResponse(http.StatusOK, bad), Payload: map[string]any{"prompt": "original"}, Request: std}, Options{RetryEnabled: true})
	if outErr != nil || len(result.Turn.ToolCalls) != 1 {
		t.Fatalf("tool output not recovered: err=%+v turn=%+v", outErr, result.Turn)
	}
	if len(ds.payloads) != 1 || !strings.Contains(ds.payloads[0]["prompt"].(string), ToolOutputRetrySuffix) || !strings.Contains(ds.payloads[0]["prompt"].(string), `"code":"invalid_tool_arguments"`) || !strings.Contains(ds.payloads[0]["prompt"].(string), "query") {
		t.Fatal("missing corrective tool retry prompt")
	}
}

func TestNonStreamPreservesTransientSSEErrorForFailover(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"keys":["managed"],"accounts":[{"email":"a@test.com","token":"ta"},{"email":"b@test.com","token":"tb"}]}`)
	store := config.LoadStore()
	r := auth.NewResolver(store, account.NewPool(store), func(_ context.Context, acc config.Account) (string, error) { return "token-" + acc.Identifier(), nil })
	req, _ := http.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer managed")
	a, err := r.Determine(req)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Release(a)
	first := a.AccountID
	ds := &fakeDeepSeekCaller{sessionByAccount: true, responses: []*http.Response{sseHTTPResponse(http.StatusOK, `data: {"p":"response/content","v":"recovered"}`)}}
	result, outErr := ExecuteNonStreamStartedWithRetry(context.Background(), ds, a, StartResult{Response: sseHTTPResponse(http.StatusOK, `data: {"error":{"message":"服务器暂时不可用"}}`), Payload: map[string]any{"prompt": "original"}}, Options{RetryEnabled: true})
	if outErr != nil || result.Turn.Text != "recovered" || a.AccountID == first {
		t.Fatalf("transient SSE failure was not switched: err=%+v result=%+v", outErr, result)
	}
	if strings.Contains(ds.payloads[0]["prompt"].(string), EmptyOutputRetrySuffix) {
		t.Fatal("explicit upstream failure was incorrectly treated as empty output")
	}
}

func TestRetryPolicyDoesNotRegenerateDeliveredOrPermanentErrors(t *testing.T) {
	for _, code := range []string{"content_filter", "input_content_blocked", "error", "upstream_error"} {
		if CanDeferOutputError(&assistantturn.OutputError{Status: 400, Code: code, Message: "内容超长"}, true, false) {
			t.Fatalf("permanent error %s was retried", code)
		}
	}
	for _, code := range []string{"invalid_tool_arguments", "tool_choice_violation", "upstream_unavailable"} {
		err := &assistantturn.OutputError{Code: code}
		if !CanDeferOutputError(err, true, false) || CanDeferOutputError(err, true, true) || CanDeferOutputError(err, false, false) {
			t.Fatalf("incorrect regeneration policy for %s", code)
		}
	}
}
