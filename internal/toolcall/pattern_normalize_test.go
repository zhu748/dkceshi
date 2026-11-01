package toolcall

import (
	"reflect"
	"testing"
)

func TestPatternPropertyNormalizationUsesEveryMatchingConstraint(t *testing.T) {
	schema := map[string]any{"type": "object", "patternProperties": map[string]any{
		"^id_":     map[string]any{"type": "string"},
		"_nested$": map[string]any{"type": "object", "properties": map[string]any{"value": map[string]any{"type": "string"}}},
	}, "additionalProperties": false}
	tools := []any{map[string]any{"name": "read", "parameters": schema}}
	calls := NormalizeParsedToolCallsForSchemas([]ParsedToolCall{{Name: "read", Input: map[string]any{"id_x": 123, "item_nested": map[string]any{"value": 456}}}}, tools)
	want := map[string]any{"id_x": "123", "item_nested": map[string]any{"value": "456"}}
	if !reflect.DeepEqual(calls[0].Input, want) {
		t.Fatalf("got=%+v", calls)
	}
	if err := ValidateToolArguments(calls, tools); err != nil {
		t.Fatal(err)
	}
	schema["patternProperties"] = map[string]any{"^id_": map[string]any{"type": "string"}, "x$": map[string]any{"type": "number"}}
	calls = NormalizeParsedToolCallsForSchemas([]ParsedToolCall{{Name: "read", Input: map[string]any{"id_x": 123}}}, tools)
	if err := ValidateToolArguments(calls, tools); err == nil {
		t.Fatal("conflicting matching patterns bypassed validation")
	}
}
