package toolcall

import (
	"context"
	"testing"
)

func TestLiteralStringsFollowReferencesAndCompositions(t *testing.T) {
	for _, schema := range []map[string]any{
		{"$defs": map[string]any{"text": map[string]any{"type": "string"}}, "properties": map[string]any{"payload": map[string]any{"$ref": "#/$defs/text"}}},
		{"allOf": []any{map[string]any{"properties": map[string]any{"payload": map[string]any{"type": "string"}}}}},
		{"patternProperties": map[string]any{"^payload$": map[string]any{"type": "string"}}},
		{"additionalProperties": map[string]any{"type": "string"}},
	} {
		raw := `<tool_calls><invoke name="write"><parameter name="payload"><![CDATA[{"a": 1}]]></parameter></invoke></tool_calls>`
		tools := []any{map[string]any{"name": "write", "parameters": schema}}
		calls := NormalizeParsedToolCallsForSchemas(ParseToolCalls(raw, nil), tools)
		if len(calls) != 1 || calls[0].Input["payload"] != `{"a": 1}` {
			t.Fatalf("literal lost for schema=%v calls=%+v", schema, calls)
		}
		if err := ValidateToolArguments(calls, tools); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLegacyStructuredCDATAAndCustomInputs(t *testing.T) {
	raw := `<tool_calls><invoke name="write"><parameter name="payload"><![CDATA[{"a": 1}]]></parameter></invoke></tool_calls>`
	tools := []any{map[string]any{"name": "write", "parameters": map[string]any{"properties": map[string]any{"payload": map[string]any{"type": "object"}}}}}
	calls := NormalizeParsedToolCallsForSchemas(ParseToolCalls(raw, nil), tools)
	if _, ok := calls[0].Input["payload"].(map[string]any); !ok {
		t.Fatal("legacy object became string")
	}
	raw = `<tool_calls><invoke name="custom"><parameter name="input"><![CDATA["hello"]]></parameter></invoke></tool_calls>`
	tools = []any{map[string]any{"name": "custom", "type": "custom"}}
	calls = NormalizeParsedToolCallsForSchemas(ParseToolCalls(raw, nil), tools)
	if calls[0].Input["input"] != `"hello"` {
		t.Fatal("custom input changed")
	}
	if err := ValidateToolArguments(calls, tools); err != nil {
		t.Fatal(err)
	}
}

func TestToolResourceBudgetsRejectBeforeInvocation(t *testing.T) {
	t.Setenv("DS2API_STREAM_MAX_TOOL_CALLS", "1")
	if err := ValidateToolArguments([]ParsedToolCall{{Name: "read"}, {Name: "write"}}, nil); err == nil {
		t.Fatal("oversized batch accepted")
	}
	t.Setenv("DS2API_STREAM_MAX_REPAIR_BYTES", "1")
	invoked := false
	if _, ok := RepairToolCallsWithLLM(context.Background(), `<|EPSE|call name="write"></|EPSE|call>`, func(context.Context, string) (string, error) { invoked = true; return "", nil }); ok || invoked {
		t.Fatal("oversized repair invoked model")
	}
}

func TestNestedCDATAStringsAndTypedLeaves(t *testing.T) {
	schema := map[string]any{"type": "object", "properties": map[string]any{"options": map[string]any{"type": "object", "properties": map[string]any{"label": map[string]any{"type": "string"}, "enabled": map[string]any{"type": "boolean"}}}}}
	tools := []any{map[string]any{"name": "write", "parameters": schema}}
	raw := `<tool_calls><invoke name="write"><parameter name="options"><label><![CDATA["hello"]]></label><enabled><![CDATA[true]]></enabled></parameter></invoke></tool_calls>`
	calls := NormalizeParsedToolCallsForSchemas(ParseToolCalls(raw, nil), tools)
	if err := ValidateToolArguments(calls, tools); err != nil {
		t.Fatal(err)
	}
	options := calls[0].Input["options"].(map[string]any)
	if options["label"] != `"hello"` || options["enabled"] != true {
		t.Fatalf("nested literal lost: %+v", options)
	}
}

func TestNestedCDATAKeepsCarriageReturnsAndSplitCDATA(t *testing.T) {
	raw := `<tool_calls><invoke name="write"><parameter name="options"><label><![CDATA[  "hello"` + "\r\n\t" + `]]]]><![CDATA[>tail]]></label></parameter></invoke></tool_calls>`
	calls := ParseToolCalls(raw, nil)
	got := calls[0].Input["options"].(map[string]any)["label"]
	if got != "  \"hello\"\r\n\t]]>tail" {
		t.Fatalf("nested XML normalized original bytes: %q", got)
	}
}
