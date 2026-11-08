package toolcall

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestConditionalSchemaNormalizesOnlyTheSelectedBranch(t *testing.T) {
	schema := map[string]any{"type": "object", "properties": map[string]any{"kind": map[string]any{"type": "string"}}, "if": map[string]any{"properties": map[string]any{"kind": map[string]any{"const": "n"}}, "required": []any{"kind"}}, "then": map[string]any{"properties": map[string]any{"value": map[string]any{"type": "integer"}}, "required": []any{"value"}}, "else": map[string]any{"properties": map[string]any{"value": map[string]any{"type": "boolean"}}, "required": []any{"value"}}}
	tools := []any{map[string]any{"name": "example", "parameters": schema}}
	for _, tc := range []struct {
		kind, value string
		want        any
	}{{"n", "42", float64(42)}, {"n", "9007199254740993", json.Number("9007199254740993")}, {"b", "true", true}} {
		source := map[string]any{"kind": tc.kind, "value": tc.value}
		calls := NormalizeParsedToolCallsForSchemas([]ParsedToolCall{{Name: "example", Input: source}}, tools)
		if err := ValidateToolArguments(calls, tools); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(calls[0].Input["value"], tc.want) {
			t.Fatalf("wrong coercion: %#v", calls[0].Input)
		}
		if source["value"] != tc.value {
			t.Fatal("original arguments changed")
		}
	}
}

func TestConditionalNormalizationDoesNotFlipDiscriminator(t *testing.T) {
	schema := map[string]any{"type": "object", "if": map[string]any{"properties": map[string]any{"kind": map[string]any{"type": "string"}}, "required": []any{"kind"}}, "then": map[string]any{"properties": map[string]any{"kind": map[string]any{"type": "integer"}}}}
	tools := []any{map[string]any{"name": "example", "parameters": schema}}
	source := map[string]any{"kind": "42"}
	calls := NormalizeParsedToolCallsForSchemas([]ParsedToolCall{{Name: "example", Input: source}}, tools)
	if !reflect.DeepEqual(calls[0].Input, source) || ValidateToolArguments(calls, tools) == nil {
		t.Fatal("changed discriminator to escape the selected branch")
	}
}

func TestDependentSchemaRecoversValuesAndPreservesConditionalCDATA(t *testing.T) {
	schema := map[string]any{"type": "object", "dependentSchemas": map[string]any{"enabled": map[string]any{"properties": map[string]any{"count": map[string]any{"type": "integer"}, "literal": map[string]any{"type": "string"}}, "required": []any{"count", "literal"}}}}
	tools := []any{map[string]any{"name": "example", "parameters": schema}}
	source := `<tool_calls><invoke name="example"><parameter name="enabled">true</parameter><parameter name="count"><![CDATA[42]]></parameter><parameter name="literal"><![CDATA[  42  ]]></parameter></invoke></tool_calls>`
	calls := NormalizeParsedToolCallsForSchemas(ParseToolCalls(source, nil), tools)
	if err := ValidateToolArguments(calls, tools); err != nil {
		t.Fatal(err)
	}
	if calls[0].Input["literal"] != "  42  " {
		t.Fatalf("conditional string lost bytes: %#v", calls[0].Input)
	}
}
