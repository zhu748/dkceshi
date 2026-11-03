package promptcompat

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLegacyHistoryNormalizationLinksResultsWithoutMutatingInput(t *testing.T) {
	messages := []any{
		map[string]any{"role": "assistant", "function_call": map[string]any{"name": "read_file", "arguments": `{"path":"file.go"}`}},
		map[string]any{"role": "function", "name": "read_file", "content": "result"},
		historyCall("read_file", "call_legacy_1"),
		historyResult("read_file", "call_legacy_1"),
	}
	before, _ := json.Marshal(messages)
	for _, normalize := range []func([]any) []any{NormalizeLegacyToolHistory, func(m []any) []any {
		req, err := NormalizeOpenAIChatRequest(nil, map[string]any{"model": "deepseek-v4.1-flash", "messages": m}, "")
		if err != nil {
			t.Fatal(err)
		}
		return req.Messages
	}} {
		out := normalize(messages)
		calls := out[0].(map[string]any)["tool_calls"].([]any)
		id := calls[0].(map[string]any)["id"]
		result := out[1].(map[string]any)
		if id == "call_legacy_1" || result["tool_call_id"] != id || result["role"] != "tool" {
			t.Fatal("legacy identity lost or collided")
		}
		if err := ValidateToolHistory(out); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(BuildOpenAIHistoryTranscript(out), `call_id="`+id.(string)+`"`) {
			t.Fatal("history lost legacy call ID")
		}
	}
	after, _ := json.Marshal(messages)
	if string(before) != string(after) {
		t.Fatal("caller messages mutated")
	}
}

func TestResponsesRetainsLegacyCallsAndTheirResults(t *testing.T) {
	input := []any{
		map[string]any{"role": "assistant", "id": "legacy_id", "function_call": map[string]any{"name": "lookup", "arguments": `{"query":"needle"}`}},
		map[string]any{"role": "function", "name": "lookup", "tool_call_id": "legacy_id", "content": "found"},
	}
	req, err := NormalizeOpenAIResponsesRequest(nil, map[string]any{"model": "deepseek-v4.1-flash", "input": input}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(req.FinalPrompt, `call_id="legacy_id"`) || !strings.Contains(req.FinalPrompt, "needle") {
		t.Fatal("Responses lost legacy invocation")
	}
}

func TestReadGuardPreservesCompleteSourceAndExplicitSuccess(t *testing.T) {
	for _, body := range []string{"func run() error { return nil }", "cached := true", "// permission denied\nreturn value", strings.Repeat("source ", 100) + "error"} {
		if ReadToolResultNeedsCacheGuard([]any{map[string]any{"role": "tool", "name": "read_file", "content": body}}) {
			t.Fatalf("file mistaken for status: %q", body)
		}
	}
	if ReadToolResultNeedsCacheGuard([]any{map[string]any{"role": "tool", "name": "read_file", "is_error": false, "content": "error opening file"}}) {
		t.Fatal("explicit successful body treated as error")
	}
	if !ReadToolResultNeedsCacheGuard([]any{map[string]any{"role": "tool", "name": "read_file", "is_error": true, "content": "custom failure\nwith details"}}) {
		t.Fatal("structured failure ignored")
	}
}
