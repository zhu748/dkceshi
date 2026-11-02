package toolcall

import (
	"reflect"
	"strings"
	"testing"
)

func TestArrayCloseRecoveryFollowsCompiledReferences(t *testing.T) {
	const raw = `[{"id":9007199254740993,"text":"[literal] &amp;"}`
	array := map[string]any{"type": "array", "items": map[string]any{"type": "object"}}
	for _, field := range []any{
		array,
		map[string]any{"$ref": "#/$defs/list"},
		map[string]any{"allOf": []any{array}},
		map[string]any{"anyOf": []any{array, map[string]any{"type": "null"}}},
	} {
		schema := map[string]any{"type": "object", "$defs": map[string]any{"list": array}, "properties": map[string]any{"payload": field}}
		tools := []any{map[string]any{"name": "choose", "parameters": schema}}
		calls := []ParsedToolCall{{Name: "choose", Input: map[string]any{"payload": raw}}}
		got := NormalizeParsedToolCallsForSchemas(calls, tools)
		want, ok := parseJSONLiteralValue(raw + "]")
		if !ok || !reflect.DeepEqual(got[0].Input["payload"], want) {
			t.Fatalf("field=%v: unexpected recovered value: %#v", field, got)
		}
		if err := ValidateToolArguments(got, tools); err != nil {
			t.Fatal(err)
		}
		if calls[0].Input["payload"] != raw {
			t.Fatal("normalization mutated the source input")
		}
	}
}

func TestArrayCloseRecoveryPreservesStringAndUnknownFields(t *testing.T) {
	const raw = `[{"id":"q","question":"原文"}`
	for _, field := range []any{
		map[string]any{"type": "string"},
		map[string]any{"type": "string", "pattern": "^prefix"},
		map[string]any{"type": []any{"array", "string"}},
		map[string]any{"anyOf": []any{map[string]any{"type": "array"}, map[string]any{"type": "string"}}},
		map[string]any{},
	} {
		tools := []any{map[string]any{"name": "write", "parameters": map[string]any{"properties": map[string]any{"payload": field}}}}
		got := NormalizeParsedToolCallsForSchemas([]ParsedToolCall{{Name: "write", Input: map[string]any{"payload": raw}}}, tools)
		if got[0].Input["payload"] != raw {
			t.Fatalf("literal string changed for field=%v: %#v", field, got)
		}
	}
}

func TestNestedArrayCloseRecoveryFollowsReferences(t *testing.T) {
	const raw = `[{"id":9007199254740993,"options":[{"text":"literal ] } \" &amp;","enabled":true}`
	array := map[string]any{"type": "array", "items": map[string]any{"type": "object"}}
	for _, field := range []any{
		array,
		map[string]any{"$ref": "#/$defs/list"},
		map[string]any{"allOf": []any{array}},
		map[string]any{"oneOf": []any{array, map[string]any{"type": "null"}}},
	} {
		schema := map[string]any{"$defs": map[string]any{"list": array}, "properties": map[string]any{"payload": field}}
		tools := []any{map[string]any{"name": "choose", "parameters": schema}}
		calls := []ParsedToolCall{{Name: "choose", Input: map[string]any{"payload": raw}}}
		got := NormalizeParsedToolCallsForSchemas(calls, tools)
		want, ok := parseJSONLiteralValue(raw + `]}]`)
		if !ok || !reflect.DeepEqual(got[0].Input["payload"], want) {
			t.Fatalf("field=%v: nested values changed: %#v", field, got)
		}
		if err := ValidateToolArguments(got, tools); err != nil {
			t.Fatal(err)
		}
		if calls[0].Input["payload"] != raw {
			t.Fatal("source input changed")
		}
	}
}

func TestArrayCloseRecoveryRejectsOtherSyntaxDamage(t *testing.T) {
	for _, raw := range []string{
		`[{"x":"unclosed}`, `[{"x":"escaped\"}`, `[{"x":"complete"`,
		`[{"x":}]`, `[{"x":1},`, `[{"x":1]`, `[{"x":1} trailing }`,
		`[{"x":1e}`, `[{"x":1}{} `, `[{"x":1}]`, `[`, `[1`, `{"x":[{}`,
		strings.Repeat("[", 129) + `{}`,
	} {
		if _, ok := parseArrayMissingClosures(raw); ok {
			t.Fatalf("unexpected recovery for %q", raw)
		}
	}
}

func TestNestedArrayCloseRecoveryPreservesSchemaConstraints(t *testing.T) {
	const raw = `[{"id":"q","options":[{"label":"x"}`
	for _, field := range []any{
		map[string]any{"type": "string"},
		map[string]any{"type": []any{"array", "string"}},
		map[string]any{},
		map[string]any{"type": "array", "minItems": 2},
		map[string]any{"type": "array", "items": map[string]any{"required": []any{"question"}}},
	} {
		tools := []any{map[string]any{"name": "choose", "parameters": map[string]any{"properties": map[string]any{"payload": field}}}}
		got := NormalizeParsedToolCallsForSchemas([]ParsedToolCall{{Name: "choose", Input: map[string]any{"payload": raw}}}, tools)
		if got[0].Input["payload"] != raw {
			t.Fatalf("invalid or literal value changed for schema=%v", field)
		}
	}
}
