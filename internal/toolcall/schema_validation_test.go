package toolcall

import "testing"

func TestSchemaValidationChecksNestedConstraintsAndLocalReferences(t *testing.T) {
	tools := []any{map[string]any{"name": "lookup", "parameters": map[string]any{"type": "object", "properties": map[string]any{"item": map[string]any{"$ref": "#/$defs/item"}}, "required": []any{"item"}, "additionalProperties": false, "$defs": map[string]any{"item": map[string]any{"type": "object", "properties": map[string]any{"mode": map[string]any{"enum": []any{"a", "b"}}}, "required": []any{"mode"}, "additionalProperties": false}}}}}
	for _, input := range []map[string]any{{}, {"item": map[string]any{"mode": "wrong"}}, {"item": map[string]any{"mode": "a", "extra": true}}, {"item": "a"}} {
		if err := ValidateToolArguments([]ParsedToolCall{{Name: "lookup", Input: input}}, tools); err == nil {
			t.Fatalf("invalid input accepted: %#v", input)
		}
	}
	if err := ValidateToolArguments([]ParsedToolCall{{Name: "lookup", Input: map[string]any{"item": map[string]any{"mode": "a"}}}}, tools); err != nil {
		t.Fatal(err)
	}
}
func TestSchemaValidationNeverLoadsExternalResources(t *testing.T) {
	for _, ref := range []string{"file:///etc/passwd", "https://example.invalid/schema"} {
		if _, err := compileToolSchema(map[string]any{"$ref": ref}); err == nil {
			t.Fatalf("external reference accepted: %s", ref)
		}
	}
}
func TestSchemaNormalizationTreatsToolNamesAsCaseSensitive(t *testing.T) {
	tools := []any{map[string]any{"name": "Read", "parameters": map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}}}}, map[string]any{"name": "read", "parameters": map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "number"}}}}}
	calls := NormalizeParsedToolCallsForSchemas([]ParsedToolCall{{Name: "Read", Input: map[string]any{"id": 1}}, {Name: "read", Input: map[string]any{"id": 1}}}, tools)
	if calls[0].Input["id"] != "1" || calls[1].Input["id"] != 1 {
		t.Fatal("case-sensitive tool schemas collided")
	}
}
