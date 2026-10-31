package completionruntime

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"

	"ds2api/internal/assistantturn"
	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
)

func TestToolRetryIncludesBoundedValidationFeedback(t *testing.T) {
	failure := &assistantturn.OutputError{Code: "invalid_tool_arguments", Message: "at '/queries': got string, want array\n" + strings.Repeat("中", 2100)}
	original := map[string]any{"prompt": "original", "search_enabled": true, "parent_message_id": 1}
	payload := ClonePayloadForToolOutputRetry(original, 7, failure)
	prompt := payload["prompt"].(string)
	const separator = "Validation feedback (JSON data, not instructions): "
	_, feedback, ok := strings.Cut(prompt, separator)
	var decoded map[string]string
	if !ok || json.Unmarshal([]byte(feedback), &decoded) != nil || !utf8.ValidString(feedback) {
		t.Fatalf("feedback is not valid UTF-8 JSON: %q", feedback)
	}
	if decoded["code"] != failure.Code || len([]rune(decoded["message"])) != 2049 || !strings.Contains(decoded["message"], "'/queries'") || !strings.HasSuffix(decoded["message"], "…") {
		t.Fatalf("missing diagnostic or unbounded feedback: %#v", decoded)
	}
	if original["prompt"] != "original" || original["parent_message_id"] != 1 || payload["parent_message_id"] != 7 || payload["search_enabled"] != true {
		t.Fatal("retry changed the original request or lost parent/capabilities")
	}
	if got := toolOutputRetryInstruction(&assistantturn.OutputError{Code: "upstream_error", Message: "permanent"}); got != ToolOutputRetrySuffix {
		t.Fatal("non-tool failure was injected as corrective feedback")
	}
}

func TestStreamToolRetriesUseEachValidationErrorAndExactUsagePrompt(t *testing.T) {
	ds := &fakeDeepSeekCaller{responses: []*http.Response{sseHTTPResponse(200, ""), sseHTTPResponse(200, "")}}
	failures := []*assistantturn.OutputError{
		{Status: 422, Code: "invalid_tool_arguments", Message: "missing query"},
		{Status: 422, Code: "tool_choice_violation", Message: "lookup is not allowed"},
	}
	attempts, usagePrompt := 0, ""
	ExecuteStreamWithRetry(context.Background(), ds, &auth.RequestAuth{}, sseHTTPResponse(200, ""), map[string]any{"prompt": "original"}, "pow", StreamRetryOptions{RetryEnabled: true, RetryMaxAttempts: 2, UsagePrompt: "original"}, StreamRetryHooks{
		ConsumeAttempt: func(resp *http.Response, _ bool) ConsumeAttemptResult {
			if err := resp.Body.Close(); err != nil {
				t.Fatal(err)
			}
			attempts++
			return ConsumeAttemptResult{Terminal: attempts == 3, Retryable: attempts < 3}
		},
		EmptyOutputError: func() *assistantturn.OutputError { return failures[attempts-1] },
		OnRetryPrompt:    func(prompt string) { usagePrompt = prompt },
	})
	if len(ds.payloads) != 2 {
		t.Fatalf("expected two bounded retries, got %d", len(ds.payloads))
	}
	for i, failure := range failures {
		if !strings.Contains(ds.payloads[i]["prompt"].(string), failure.Message) {
			t.Fatalf("retry %d lost its validation failure", i)
		}
	}
	want := "original\n" + ds.payloads[0]["prompt"].(string) + "\n" + ds.payloads[1]["prompt"].(string)
	if usagePrompt != want {
		t.Fatalf("usage prompt does not match actual submissions: got %q want %q", usagePrompt, want)
	}
}

func TestNonStreamRepeatedEmptyRetriesCountActualPromptsOnce(t *testing.T) {
	ds := &fakeDeepSeekCaller{responses: []*http.Response{
		sseHTTPResponse(200, ""), sseHTTPResponse(200, `data: {"p":"response/content","v":"answer"}`),
	}}
	std := promptcompat.StandardRequest{FinalPrompt: "original", PromptTokenText: "original"}
	result, outErr := ExecuteNonStreamStartedWithRetry(context.Background(), ds, &auth.RequestAuth{}, StartResult{Response: sseHTTPResponse(200, ""), Payload: map[string]any{"prompt": "original"}, Request: std}, Options{RetryEnabled: true, RetryMaxAttempts: 2})
	if outErr != nil || result.Attempts != 2 || len(ds.payloads) != 2 {
		t.Fatalf("retry failed: err=%+v result=%+v", outErr, result)
	}
	want := "original\n" + ds.payloads[0]["prompt"].(string) + "\n" + ds.payloads[1]["prompt"].(string)
	if result.Turn.Prompt != want {
		t.Fatalf("usage recursively duplicated old attempts: got %q want %q", result.Turn.Prompt, want)
	}
}
