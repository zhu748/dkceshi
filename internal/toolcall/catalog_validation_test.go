package toolcall

import "testing"

func TestMalformedCatalogNeverBecomesUnrestricted(t *testing.T) {
	for _, raw := range []any{
		"tools", []any{nil}, []any{map[string]any{}},
		[]any{map[string]any{"type": "function", "function": map[string]any{"parameters": map[string]any{"type": "object"}}}},
		[]any{map[string]any{"name": "a", "function": map[string]any{"name": "b"}}},
		[]any{map[string]any{"name": "a", "function": "invalid"}},
		[]any{map[string]any{"type": "unsupported", "name": "a"}},
		[]any{map[string]any{"name": "a"}, map[string]any{"name": "a"}},
	} {
		if err := ValidateToolSchemas(raw); err == nil {
			t.Fatalf("malformed catalog accepted: %#v", raw)
		}
	}
	for _, raw := range []any{nil, []any{}, []any{map[string]any{"name": "zero_args"}}, []any{map[string]any{"type": "custom", "name": "freeform"}}, []any{map[string]any{"type": "function", "function": map[string]any{"name": "a", "parameters": map[string]any{"type": "object"}}}}} {
		if err := ValidateToolSchemas(raw); err != nil {
			t.Fatalf("valid normalized catalog rejected: %v", err)
		}
	}
}

func TestCompiledNormalizationHandlesReferencesUnionsAndRecursiveSchemas(t *testing.T) {
	root := map[string]any{
		"type": "object", "$defs": map[string]any{"text": map[string]any{"type": "string"}},
		"properties": map[string]any{
			"path":   map[string]any{"$ref": "#/$defs/text"},
			"tags":   map[string]any{"type": "array", "prefixItems": []any{map[string]any{"$ref": "#/$defs/text"}}},
			"union":  map[string]any{"anyOf": []any{map[string]any{"type": "string"}, map[string]any{"type": "number"}}},
			"object": map[string]any{"anyOf": []any{map[string]any{"type": "null"}, map[string]any{"type": "object", "properties": map[string]any{"value": map[string]any{"$ref": "#/$defs/text"}}}}},
			"child":  map[string]any{"$ref": "#"},
		},
	}
	tools := []any{map[string]any{"name": "work", "parameters": root}}
	input := map[string]any{"path": 123, "tags": []any{456}, "union": 789, "object": map[string]any{"value": 42}, "child": map[string]any{"path": 7}}
	calls := NormalizeParsedToolCallsForSchemas([]ParsedToolCall{{Name: "work", Input: input}}, tools)
	got := calls[0].Input
	if got["path"] != "123" || got["tags"].([]any)[0] != "456" || got["union"] != 789 || got["child"].(map[string]any)["path"] != "7" || got["object"].(map[string]any)["value"] != "42" {
		t.Fatalf("compiled schema coercion mismatch: %#v", got)
	}
	if input["path"] != 123 {
		t.Fatal("caller-owned input mutated")
	}
	if err := ValidateToolArguments(calls, tools); err != nil {
		t.Fatal(err)
	}
	cycle := map[string]any{"$ref": "#"}
	if _, err := compileToolSchema(cycle); err == nil {
		if _, changed := normalizeToolValueWithSchema(map[string]any{}, cycle); changed {
			t.Fatal("schema-only cycle changed arguments")
		}
	}
}
