package toolcall

import "testing"

func TestPermissiveSchemaDoesNotChangeCDATAStringType(t *testing.T) {
	for _, schema := range []any{
		map[string]any{"type": "object"},
		map[string]any{"type": "object", "properties": map[string]any{"mode": map[string]any{}}},
		map[string]any{"type": "object", "additionalProperties": true},
	} {
		parsed := ParseToolCalls(`<tool_calls><invoke name="check"><parameter name="mode"><![CDATA[true]]></parameter></invoke></tool_calls>`, nil)
		tools := []any{map[string]any{"name": "check", "parameters": schema}}
		calls := NormalizeParsedToolCallsForSchemas(parsed, tools)
		if len(calls) != 1 || calls[0].Input["mode"] != "true" {
			t.Fatalf("schema without a type constraint changed string: %#v", calls)
		}
		if err := ValidateToolArguments(calls, tools); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLiteralRestorationRespectsUnevaluatedPropertyType(t *testing.T) {
	tools := []any{map[string]any{"name": "check", "parameters": map[string]any{"type": "object", "unevaluatedProperties": map[string]any{"type": "integer"}}}}
	parsed := ParseToolCalls(`<tool_calls><invoke name="check"><parameter name="mode"><![CDATA[42]]></parameter></invoke></tool_calls>`, nil)
	calls := NormalizeParsedToolCallsForSchemas(parsed, tools)
	if err := ValidateToolArguments(calls, tools); err != nil {
		t.Fatalf("explicit integer constraint lost during literal restoration: %v", err)
	}
	if _, ok := calls[0].Input["mode"].(string); ok {
		t.Fatal("explicit non-string constraint was ignored")
	}
}
