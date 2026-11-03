package promptcompat

import (
	"strings"
	"testing"
)

func TestRegressionSuccessfulReadNotMarkedMissing(t *testing.T) {
	messages := []any{
		map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "r1", "function": map[string]any{"name": "read_file", "arguments": `{"path":"main.go"}`}}}},
		map[string]any{"role": "tool", "tool_call_id": "r1", "content": "package main\nfunc run() error { return nil }\n// complete file contents"},
	}
	if ReadToolResultNeedsCacheGuard(messages) {
		t.Fatal("complete valid source file misclassified as missing body because it contains error")
	}
}

func TestRegressionSplitPreservesRepeatedIDDependencies(t *testing.T) {
	messages := []any{
		map[string]any{"role": "system", "content": "instructions"},
		map[string]any{"role": "user", "content": "old task"},
		historyCall("read_file", "same_id"),
		map[string]any{"role": "user", "content": "new task"},
		historyResult("read_file", "same_id"),
		historyCall("write_file", "same_id"),
		historyResult("write_file", "same_id"),
	}
	if err := ValidateToolHistory(messages); err != nil {
		t.Fatal(err)
	}
	history, live := SplitActiveContext(messages)
	if strings.Contains(RenderPromptText(history, false), `name="read_file"`) && strings.Contains(RenderPromptText(live, false), `"tool_call_id":"same_id"`) {
		t.Fatal("accepted reused ID causes original call and its active result to be split between HISTORY and live prompt")
	}
}

func TestRegressionLegacyFunctionCallRetained(t *testing.T) {
	messages := []any{map[string]any{"role": "assistant", "content": nil, "function_call": map[string]any{"name": "lookup_legacy", "arguments": `{"query":"needle"}`}}, map[string]any{"role": "function", "name": "lookup_legacy", "content": "result"}}
	text := RenderPromptText(messages, false)
	if !strings.Contains(text, "needle") {
		t.Fatal("legacy function_call and its arguments disappeared from transcript")
	}
}

func TestRegressionHistoryNormalizationPreservesUnparsedSibling(t *testing.T) {
	raw := `<|EPSE|tool_calls><|EPSE|invoke name="read"></|EPSE|invoke><|EPSE|call name="write"><|EPSE|parameter name="path">IMPORTANT_FILE.txt</|EPSE|parameter></|EPSE|call></|EPSE|tool_calls>`
	text := RenderPromptText([]any{map[string]any{"role": "assistant", "content": raw}}, false)
	if !strings.Contains(text, "IMPORTANT_FILE.txt") {
		t.Fatalf("history canonicalization silently discarded broken write sibling; result=%s", text)
	}
}

func TestRegressionHistoryNormalizationPreservesCallID(t *testing.T) {
	raw := `<|EPSE|tool_calls><|EPSE|invoke name="read" call_id="original_id"><|EPSE|parameter name="path"><![CDATA[file.txt]]></|EPSE|parameter></|EPSE|invoke></|EPSE|tool_calls>`
	text := RenderPromptText([]any{map[string]any{"role": "assistant", "content": raw}}, false)
	if !strings.Contains(text, "original_id") {
		t.Fatal("canonicalization removed existing call_id from raw tool history")
	}
}
