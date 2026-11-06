package toolcall

import (
	"fmt"
	"reflect"
	"testing"
)

func TestRepeatedCDATAKeepsEveryStringOccurrence(t *testing.T) {
	for _, items := range []map[string]any{
		{"type": "string"},
		{"$ref": "#/$defs/text"},
		{"anyOf": []any{map[string]any{"type": "string"}, map[string]any{"type": "integer"}}},
	} {
		schema := map[string]any{"type": "object", "$defs": map[string]any{"text": map[string]any{"type": "string"}}, "properties": map[string]any{"labels": map[string]any{"type": "array", "items": items}}}
		tools := []any{map[string]any{"name": "write", "parameters": schema}}
		raw := `<tool_calls><invoke name="write"><parameter name="labels"><![CDATA[ 1 ]]></parameter><parameter name="labels"><![CDATA[ "two" ]]></parameter><parameter name="labels"><![CDATA[ null ]]></parameter></invoke></tool_calls>`
		parsed := ParseToolCalls(raw, nil)
		for _, catalog := range []any{nil, tools} {
			calls := NormalizeParsedToolCallsForSchemas(parsed, catalog)
			if len(calls) != 1 || !reflect.DeepEqual(calls[0].Input["labels"], []any{" 1 ", ` "two" `, " null "}) {
				t.Fatalf("repeated CDATA was rewritten: %+v", calls)
			}
			if err := ValidateToolArguments(calls, catalog); err != nil {
				t.Fatal(err)
			}
		}
		if reflect.DeepEqual(parsed[0].Input["labels"], []any{" 1 ", ` "two" `, " null "}) {
			t.Fatal("literal restoration mutated its source")
		}
	}
}

func TestRepeatedCDATARespectsArrayItemSchemas(t *testing.T) {
	schema := map[string]any{"properties": map[string]any{"values": map[string]any{"type": "array", "prefixItems": []any{map[string]any{"type": "string"}, map[string]any{"type": "integer"}, map[string]any{"type": "object"}}}}}
	tools := []any{map[string]any{"name": "write", "parameters": schema}}
	raw := `<tool_calls><invoke name="write"><parameter name="values"><![CDATA[ "exact" ]]></parameter><parameter name="values"><![CDATA[ 2 ]]></parameter><parameter name="values"><![CDATA[{"id":9007199254740993}]]></parameter></invoke></tool_calls>`
	calls := NormalizeParsedToolCallsForSchemas(ParseToolCalls(raw, nil), tools)
	if err := ValidateToolArguments(calls, tools); err != nil {
		t.Fatal(err)
	}
	values := calls[0].Input["values"].([]any)
	if values[0] != ` "exact" ` || values[1] != float64(2) || fmt.Sprint(values[2].(map[string]any)["id"]) != "9007199254740993" {
		t.Fatalf("item values changed: %#v", values)
	}
}

func TestMixedXMLCannotOverwriteExplicitTextField(t *testing.T) {
	for _, body := range []string{`<_text>ORIGINAL</_text>TAIL`, `HEAD<_text><![CDATA[ORIGINAL]]></_text>TAIL`} {
		raw := `<tool_calls><invoke name="write"><parameter name="payload">` + body + `</parameter></invoke></tool_calls>`
		calls := ParseToolCalls(raw, nil)
		if len(calls) != 1 || calls[0].Input["payload"] != body {
			t.Fatalf("ambiguous XML lost source data: %+v", calls)
		}
		tools := []any{map[string]any{"name": "write", "parameters": map[string]any{"properties": map[string]any{"payload": map[string]any{"type": "object"}}}}}
		if err := ValidateToolArguments(NormalizeParsedToolCallsForSchemas(calls, tools), tools); err == nil {
			t.Fatal("conflicting mixed XML became an executable object")
		}
	}
}

func TestLiteralRestorationIsAtomicForConditionalSchemas(t *testing.T) {
	schema := map[string]any{"type": "object", "properties": map[string]any{"values": map[string]any{
		"type": "array", "if": map[string]any{"contains": map[string]any{"type": "string"}},
		"then": map[string]any{"items": map[string]any{"type": "string"}},
		"else": map[string]any{"items": map[string]any{"type": "integer"}},
	}}}
	tools := []any{map[string]any{"name": "write", "parameters": schema}}
	raw := `<tool_calls><invoke name="write"><parameter name="values"><![CDATA[ 1 ]]></parameter><parameter name="values"><![CDATA[ 2 ]]></parameter></invoke></tool_calls>`
	calls := NormalizeParsedToolCallsForSchemas(ParseToolCalls(raw, nil), tools)
	if !reflect.DeepEqual(calls[0].Input["values"], []any{" 1 ", " 2 "}) {
		t.Fatalf("conditional schema changed literal array: %+v", calls)
	}
	if err := ValidateToolArguments(calls, tools); err != nil {
		t.Fatal(err)
	}
}
