package promptcompat

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestResponsesReasoningBetweenCallsStaysBetweenCalls(t *testing.T) {
	input := []any{
		map[string]any{"type": "function_call", "name": "first_tool", "call_id": "c1", "arguments": "{}"},
		map[string]any{"type": "reasoning", "summary": []any{map[string]any{"type": "summary_text", "text": "THOUGHT_AFTER_FIRST"}}},
		map[string]any{"type": "function_call", "name": "second_tool", "call_id": "c2", "arguments": "{}"},
	}
	messages := NormalizeResponsesInputAsMessages(input)
	for _, text := range []string{RenderPromptText(messages, true), BuildOpenAIHistoryTranscript(messages)} {
		first := strings.Index(text, `name="first_tool"`)
		thought := strings.Index(text, "THOUGHT_AFTER_FIRST")
		second := strings.Index(text, `name="second_tool"`)
		if first < 0 || thought <= first || second <= thought {
			t.Fatalf("later reasoning moved before previous call: %s", text)
		}
	}
}

func TestResponsesRejectsInvalidSourceFieldsBeforeTheyDisappear(t *testing.T) {
	for _, fields := range []map[string]any{
		{"instructions": map[string]any{"text": "LOST"}, "input": "valid"},
		{"input": []any{map[string]any{"type": "reasoning", "summary": []any{map[string]any{"text": 42}}}, "valid"}},
		{"input": []any{map[string]any{"type": "reasoning", "content": "LOST"}, "valid"}},
		{"input": []any{map[string]any{"type": "function_call", "name": "first", "function": map[string]any{"name": "different"}, "arguments": "{}"}, "valid"}},
		{"input": []any{map[string]any{"type": "function_call_output", "call_id": "first", "tool_call_id": "different", "output": "result"}, "valid"}},
		{"input": []any{map[string]any{"role": "assistant", "refusal": 42}, "valid"}},
		{"input": []any{map[string]any{"role": "assistant", "audio": map[string]any{"transcript": 42}}, "valid"}},
		{"input": []any{map[string]any{"role": "assistant", "reasoning_content": 42}, "valid"}},
		{"messages": "LOST", "input": "valid"},
		{"messages": []any{map[string]any{"role": "user", "content": "ONE"}}, "input": "TWO"},
		{"input": []any{map[string]any{"type": "function_call", "name": "read", "arguments": `{"id":9007199254740993}`, "input": map[string]any{"id": "different"}}, "valid"}},
		{"input": []any{map[string]any{"type": "function_call", "role": "user", "name": "read", "arguments": "{}"}, "valid"}},
		{"input": []any{map[string]any{"type": "input_text", "role": "user", "text": 42}, "valid"}},
	} {
		fields["model"] = "deepseek-v4.1-flash"
		if std, err := NormalizeOpenAIResponsesRequest(nil, fields, ""); err == nil {
			t.Errorf("invalid source accepted: %#v prompt=%s", fields, std.FinalPrompt)
		}
	}
}

func TestResponsesNativeItemsKeepPayloadsWithExplicitRoles(t *testing.T) {
	input := []any{
		map[string]any{"type": "reasoning", "role": "assistant", "summary": []any{map[string]any{"text": "SOURCE_THOUGHT"}}},
		map[string]any{"type": "function_call", "role": "assistant", "name": "lookup", "call_id": "c1", "arguments": `{"id":9007199254740993}`},
		map[string]any{"type": "function_call_output", "role": "tool", "call_id": "c1", "output": "SOURCE_TOOL_OUTPUT"},
		map[string]any{"type": "web_search_call", "role": "assistant", "action": map[string]any{"query": "SOURCE_HOSTED_TOOL"}},
	}
	std, err := NormalizeOpenAIResponsesRequest(nil, map[string]any{"model": "deepseek-v4.1-flash", "input": input}, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{std.FinalPrompt, BuildOpenAIHistoryTranscript(std.Messages)} {
		for _, want := range []string{"SOURCE_THOUGHT", `name="lookup"`, "9007199254740993", "SOURCE_TOOL_OUTPUT", "SOURCE_HOSTED_TOOL"} {
			if !strings.Contains(text, want) {
				t.Fatalf("explicit role discarded native item %s: %s", want, text)
			}
		}
	}
}

func TestEquivalentToolArgumentAliasesPreserveExactNumbers(t *testing.T) {
	call := map[string]any{"name": "read", "arguments": ` {"id":9007199254740993} `, "input": map[string]any{"id": json.Number("9007199254740993")}}
	if err := validateToolArgumentAliases(call, nil); err != nil {
		t.Fatalf("equivalent aliases rejected: %v", err)
	}
	call["input"] = map[string]any{"id": json.Number("9007199254740992")}
	if err := validateToolArgumentAliases(call, nil); err == nil {
		t.Fatal("different large IDs were treated as equivalent")
	}
	for _, field := range []string{"arguments", "input"} {
		message := historyCall("read", "c1")
		rawCall := message["tool_calls"].([]any)[0].(map[string]any)
		rawCall[field] = `{"lost":true}`
		if err := ValidateToolHistory([]any{message}); err == nil {
			t.Fatal("conflicting Chat argument alias was accepted")
		}
	}
}

func TestResponsesUntypedBusinessJSONRemainsOpaque(t *testing.T) {
	input := map[string]any{"text": map[string]any{"record": "KEEP_OPAQUE_DATA"}}
	std, err := NormalizeOpenAIResponsesRequest(nil, map[string]any{"model": "deepseek-v4.1-flash", "input": []any{input}}, "")
	if err != nil || !strings.Contains(std.FinalPrompt, "KEEP_OPAQUE_DATA") {
		t.Fatalf("business JSON was interpreted as a malformed text message: %v %s", err, std.FinalPrompt)
	}
}

func TestResponsesUnknownTypedToolRecordKeepsBusinessIDs(t *testing.T) {
	input := map[string]any{"type": "future_tool_record", "role": "tool", "call_id": "BUSINESS_ID_A", "tool_call_id": "BUSINESS_ID_B"}
	std, err := NormalizeOpenAIResponsesRequest(nil, map[string]any{"model": "deepseek-v4.1-flash", "input": []any{input}}, "")
	if err != nil || !strings.Contains(std.FinalPrompt, "BUSINESS_ID_A") || !strings.Contains(std.FinalPrompt, "BUSINESS_ID_B") {
		t.Fatalf("opaque identifiers were treated as tool protocol: %v %s", err, std.FinalPrompt)
	}
}
