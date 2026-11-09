package toolcall

import "testing"

func TestConditionalGuardAllowsIndependentContainerConversion(t *testing.T) {
	schema := map[string]any{"type": "object", "properties": map[string]any{"mode": map[string]any{"type": "string"}, "payload": map[string]any{"type": "string"}},
		"if": map[string]any{"properties": map[string]any{"mode": map[string]any{"const": "safe"}}}, "then": map[string]any{"required": []any{"payload"}}}
	tools := []any{map[string]any{"name": "example", "parameters": schema}}
	input := map[string]any{"mode": "safe", "payload": map[string]any{"x": "exact"}}
	calls := NormalizeParsedToolCallsForSchemas([]ParsedToolCall{{Name: "example", Input: input}}, tools)
	if err := ValidateToolArguments(calls, tools); err != nil || calls[0].Input["payload"] != `{"x":"exact"}` {
		t.Fatal("unrelated condition blocked a valid independent recovery", err)
	}
}

func TestConditionalGuardIgnoresUnmatchedUnionBranch(t *testing.T) {
	schema := map[string]any{"anyOf": []any{
		map[string]any{"type": "object", "properties": map[string]any{"mode": map[string]any{"const": "a"}, "payload": map[string]any{"type": "string"}}},
		map[string]any{"type": "object", "properties": map[string]any{"mode": map[string]any{"const": "b"}}, "if": map[string]any{"properties": map[string]any{"payload": map[string]any{"type": "object"}}}, "then": map[string]any{"required": []any{"confirmation"}}},
	}}
	tools := []any{map[string]any{"name": "example", "parameters": schema}}
	calls := NormalizeParsedToolCallsForSchemas([]ParsedToolCall{{Name: "example", Input: map[string]any{"mode": "a", "payload": map[string]any{"x": "exact"}}}}, tools)
	if err := ValidateToolArguments(calls, tools); err != nil {
		t.Fatal("inactive union branch blocked a valid recovery", err)
	}
}

func TestConditionalGuardProtectsRecoveredUnionBranch(t *testing.T) {
	schema := map[string]any{"anyOf": []any{
		map[string]any{"properties": map[string]any{"kind": map[string]any{"type": "integer"}}, "if": map[string]any{"properties": map[string]any{"kind": map[string]any{"type": "string"}}}, "then": map[string]any{"required": []any{"confirmation"}}},
		false,
	}}
	tools := []any{map[string]any{"name": "example", "parameters": schema}}
	calls := NormalizeParsedToolCallsForSchemas([]ParsedToolCall{{Name: "example", Input: map[string]any{"kind": "42"}}}, tools)
	if ValidateToolArguments(calls, tools) == nil || calls[0].Input["kind"] != "42" {
		t.Fatal("recovered union branch escaped its original condition")
	}
}
