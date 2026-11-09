package toolcall

import (
	"reflect"
	"testing"
)

func TestNormalizationPreservesOriginalConditionalBranches(t *testing.T) {
	condition := map[string]any{"properties": map[string]any{"kind": map[string]any{"type": "string"}}, "required": []any{"kind"}}
	constraint := map[string]any{"if": condition, "then": map[string]any{"required": []any{"confirmation"}}}
	coercion := map[string]any{"properties": map[string]any{"kind": map[string]any{"type": "integer"}}}
	for name, schema := range map[string]any{
		"direct": map[string]any{"type": "object", "properties": coercion["properties"], "if": condition, "then": constraint["then"]},
		"allOf":  map[string]any{"allOf": []any{coercion, constraint}},
		"ref":    map[string]any{"allOf": []any{coercion, map[string]any{"$ref": "#/$defs/condition"}}, "$defs": map[string]any{"condition": constraint}},
		"nested": map[string]any{"allOf": []any{map[string]any{"properties": map[string]any{"child": coercion}}, map[string]any{"properties": map[string]any{"child": constraint}}}},
	} {
		t.Run(name, func(t *testing.T) {
			input := map[string]any{"kind": "42"}
			if name == "nested" {
				input = map[string]any{"child": input}
			}
			tools := []any{map[string]any{"name": "example", "parameters": schema}}
			calls := NormalizeParsedToolCallsForSchemas([]ParsedToolCall{{Name: "example", Input: input}}, tools)
			if !reflect.DeepEqual(calls[0].Input, input) || ValidateToolArguments(calls, tools) == nil {
				t.Fatalf("normalization escaped original condition: %#v", calls[0].Input)
			}
		})
	}
}

func TestStaticDynamicReferencesNormalizeLikeRef(t *testing.T) {
	for _, reference := range []string{"$ref", "$dynamicRef"} {
		schema := map[string]any{"type": "object", "properties": map[string]any{"count": map[string]any{reference: "#/$defs/count"}}, "$defs": map[string]any{"count": map[string]any{"type": "integer"}}}
		tools := []any{map[string]any{"name": "example", "parameters": schema}}
		input := map[string]any{"count": "12"}
		calls := NormalizeParsedToolCallsForSchemas([]ParsedToolCall{{Name: "example", Input: input}}, tools)
		if err := ValidateToolArguments(calls, tools); err != nil {
			t.Fatalf("%s: %v", reference, err)
		}
		if input["count"] != "12" || calls[0].Input["count"] != float64(12) {
			t.Fatalf("%s mutated source or did not recover integer: %#v", reference, calls)
		}
	}
}

func TestDynamicReferenceToOrdinaryAnchorAndRecursiveStaticTarget(t *testing.T) {
	for _, schema := range []map[string]any{
		{"type": "object", "properties": map[string]any{"count": map[string]any{"$dynamicRef": "#count"}}, "$defs": map[string]any{"count": map[string]any{"$anchor": "count", "type": "integer"}}},
		{"$schema": "https://json-schema.org/draft/2019-09/schema", "type": "object", "properties": map[string]any{"count": map[string]any{"$recursiveRef": "#/$defs/count"}}, "$defs": map[string]any{"count": map[string]any{"type": "integer"}}},
	} {
		tools := []any{map[string]any{"name": "example", "parameters": schema}}
		if err := ValidateToolSchemas(tools); err != nil {
			t.Fatal(err)
		}
		calls := NormalizeParsedToolCallsForSchemas([]ParsedToolCall{{Name: "example", Input: map[string]any{"count": "12"}}}, tools)
		if err := ValidateToolArguments(calls, tools); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDynamicAnchorNormalizationDoesNotGuessOverrideScope(t *testing.T) {
	schema := map[string]any{
		"$id": "https://example.test/strict", "$dynamicAnchor": "node", "type": "object",
		"properties": map[string]any{"value": map[string]any{"type": "string"}},
		"allOf":      []any{map[string]any{"$ref": "#/$defs/base"}},
		"$defs": map[string]any{"base": map[string]any{
			"$id": "base", "$dynamicAnchor": "node", "type": "object",
			"properties": map[string]any{"children": map[string]any{"type": "array", "items": map[string]any{"$dynamicRef": "#node"}}},
		}},
	}
	tools := []any{map[string]any{"name": "example", "parameters": schema}}
	if err := ValidateToolSchemas(tools); err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"children": []any{map[string]any{"value": "  true  "}}}
	calls := NormalizeParsedToolCallsForSchemas([]ParsedToolCall{{Name: "example", Input: input}}, tools)
	if !reflect.DeepEqual(calls[0].Input, input) || ValidateToolArguments(calls, tools) != nil {
		t.Fatal("valid dynamically scoped string was changed")
	}
	// Independent coercion remains possible when dynamically scoped values
	// stay byte-identical, even though their effective anchor is overridden.
	schema["properties"].(map[string]any)["count"] = map[string]any{"type": "integer"}
	input["count"] = "12"
	calls = NormalizeParsedToolCallsForSchemas([]ParsedToolCall{{Name: "example", Input: input}}, tools)
	if input["count"] != "12" || calls[0].Input["count"] != float64(12) || ValidateToolArguments(calls, tools) != nil {
		t.Fatal("unchanged dynamic value blocked an independent conversion")
	}
}

func TestDynamicPointerSchemaKeepsCDATALiteralDuringOtherCoercions(t *testing.T) {
	schema := map[string]any{"$dynamicRef": "#/$defs/args", "$defs": map[string]any{"args": map[string]any{
		"type": "object", "properties": map[string]any{"payload": map[string]any{"type": "string"}, "count": map[string]any{"type": "integer"}},
	}}}
	tools := []any{map[string]any{"name": "example", "parameters": schema}}
	for _, literal := range []string{"  true  ", "  null  ", "  {\"a\":9007199254740993}  ", "  [1,2]  "} {
		raw := `<tool_calls><invoke name="example"><parameter name="payload"><![CDATA[` + literal + `]]></parameter><parameter name="count">"12"</parameter></invoke></tool_calls>`
		calls := NormalizeParsedToolCallsForSchemas(ParseToolCalls(raw, nil), tools)
		if len(calls) != 1 || calls[0].Input["payload"] != literal || ValidateToolArguments(calls, tools) != nil {
			t.Fatalf("dynamic pointer changed CDATA bytes: %#v", calls)
		}
	}
}
