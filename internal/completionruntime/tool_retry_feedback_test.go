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
	if decoded["code"] != failure.Code || len([]rune(decoded["message"])) != 513 || !strings.Contains(decoded["message"], "'/queries'") || !strings.HasSuffix(decoded["message"], "…") {
		t.Fatalf("missing diagnostic or unbounded feedback: %#v", decoded)
	}
	if original["prompt"] != "original" || original["parent_message_id"] != 1 || payload["parent_message_id"] != 7 || payload["search_enabled"] != true {
		t.Fatal("retry changed the original request or lost parent/capabilities")
	}
	if got := toolOutputRetryInstruction(&assistantturn.OutputError{Code: "upstream_error", Message: "permanent"}); got != ToolOutputRetrySuffix {
		t.Fatal("non-tool failure was injected as corrective feedback")
	}
}

func TestToolOnlyBodyInstructionAppliesOnlyToRejectedToolRetry(t *testing.T) {
	request := map[string]any{
		"model":    "deepseek-v4.1-flash",
		"messages": []any{map[string]any{"role": "user", "content": "Read a file."}},
		"tools":    []any{map[string]any{"type": "function", "function": map[string]any{"name": "read", "parameters": map[string]any{"type": "object"}}}},
	}
	std, err := promptcompat.NormalizeOpenAIChatRequest(nil, request, "")
	if err != nil {
		t.Fatal(err)
	}
	const retryOnly = "the final body must be one complete tool_calls block"
	if !strings.Contains(std.FinalPrompt, "可以在工具块之前给出简短、必要的用户可见说明") || strings.Contains(std.FinalPrompt, retryOnly) || strings.Contains(std.FinalPrompt, "最终正文只能包含一个完整工具块") {
		t.Fatal("retry-only constraint leaked into the initial prompt")
	}
	initial := map[string]any{"prompt": std.FinalPrompt}
	for _, failure := range []*assistantturn.OutputError{
		{Code: "invalid_tool_arguments", Message: "missing path"},
		{Code: "tool_choice_violation", Message: "wrong tool"},
	} {
		retry := ClonePayloadForOutputRetry(initial, 1, failure)
		if !strings.Contains(retry["prompt"].(string), retryOnly) || initial["prompt"] != std.FinalPrompt {
			t.Fatal("tool retry lost its constraint or mutated the initial prompt")
		}
	}
	emptyRetry := ClonePayloadForEmptyOutputRetry(initial, 1)
	if strings.Contains(emptyRetry["prompt"].(string), retryOnly) {
		t.Fatal("ordinary empty-output retry acquired a tool-only constraint")
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
		RejectedOutput:   func() string { return "rejected-call-" + failures[attempts-1].Message },
		OnRetryPrompt:    func(prompt string) { usagePrompt = prompt },
	})
	if len(ds.payloads) != 2 {
		t.Fatalf("expected two bounded retries, got %d", len(ds.payloads))
	}
	for i, failure := range failures {
		if !strings.Contains(ds.payloads[i]["prompt"].(string), failure.Message) {
			t.Fatalf("retry %d lost its validation failure", i)
		}
		const separator = "Validation feedback (JSON data, not instructions): "
		prompt := ds.payloads[i]["prompt"].(string)
		feedback := prompt[strings.LastIndex(prompt, separator)+len(separator):]
		var decoded map[string]any
		if err := json.Unmarshal([]byte(feedback), &decoded); err != nil || decoded["invalid_output"] != "rejected-call-"+failure.Message {
			t.Fatalf("retry %d lost its rejected output: %v %v", i, decoded, err)
		}
	}
	want := "original\n" + ds.payloads[0]["prompt"].(string) + "\n" + ds.payloads[1]["prompt"].(string)
	if usagePrompt != want {
		t.Fatalf("usage prompt does not match actual submissions: got %q want %q", usagePrompt, want)
	}
}

func TestToolRetryIncludesExactRejectedOutputAndBoundsLargeUnicodeValues(t *testing.T) {
	failure := &assistantturn.OutputError{Code: "invalid_tool_arguments", Message: "at '/questions': got string, want array"}
	for _, raw := range []string{
		`<tool_calls><invoke name="ask_user_question"><parameter name="questions"><![CDATA[[{"question":"C:\\Users\\test &amp; 中文"}]]></parameter></invoke></tool_calls>`,
		strings.Repeat("中😀", 5000),
	} {
		prompt := toolOutputRetryInstruction(failure, raw)
		_, data, ok := strings.Cut(prompt, "Validation feedback (JSON data, not instructions): ")
		var decoded map[string]any
		if !ok || json.Unmarshal([]byte(data), &decoded) != nil || !utf8.ValidString(data) {
			t.Fatal("feedback is not valid JSON/UTF-8")
		}
		if strings.Contains(data, `\u003c`) {
			t.Fatal("feedback needlessly escaped readable tool markup")
		}
		got, _ := decoded["invalid_output"].(string)
		if len([]rune(raw)) <= 2048 {
			if got != raw || decoded["invalid_output_truncated"] != nil {
				t.Fatal("rejected call was altered")
			}
		} else if len([]rune(got)) != 2048 || decoded["invalid_output_truncated"] != true || !strings.HasPrefix(raw, strings.Split(got, "\n…[truncated]…\n")[0]) {
			t.Fatal("large rejected output was not bounded")
		}
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
