package toolcall

import (
	"reflect"
	"strings"
	"testing"
)

func cdataNumberTools(field any) []any {
	return []any{map[string]any{"name": "read", "parameters": map[string]any{"type": "object", "properties": map[string]any{"offset": field, "limit": map[string]any{"type": "integer"}}, "required": []any{"offset"}}}}
}

func TestRepeatedCDATANumberRecoveryPreservesBatchAndPrecision(t *testing.T) {
	for _, marker := range []string{"", "ZBBZUX"} {
		for _, number := range []string{"120", "-12", "1.25", "1.2e+4", "9007199254740993"} {
			raw := number + "</![CDATA[" + number
			markup := `<tool_calls><invoke name="read"><parameter name="offset"><![CDATA[` + raw + `]]></parameter><parameter name="limit">55</parameter></invoke><invoke name="read"><parameter name="offset">1</parameter></invoke></tool_calls>`
			if marker != "" {
				for _, tag := range []string{"tool_calls", "invoke", "parameter"} {
					markup = strings.ReplaceAll(markup, "<"+tag, "<|"+marker+"|"+tag)
					markup = strings.ReplaceAll(markup, "</"+tag, "</|"+marker+"|"+tag)
				}
			}
			calls := ParseToolCalls(NormalizeMarkerText(markup, marker), nil)
			if len(calls) != 2 || calls[0].Input["offset"] != raw {
				t.Fatalf("sample was not parsed losslessly: %#v", calls)
			}
			tools := cdataNumberTools(map[string]any{"type": "number"})
			normalized := NormalizeParsedToolCallsForSchemas(calls, tools)
			want, _ := parseJSONLiteralValue(number)
			if !reflect.DeepEqual(normalized[0].Input["offset"], want) {
				t.Fatalf("number changed or not recovered: %#v", normalized)
			}
			if err := ValidateToolArguments(normalized, tools); err != nil {
				t.Fatal(err)
			}
			if calls[0].Input["offset"] != raw || !reflect.DeepEqual(normalized[1], calls[1]) || !reflect.DeepEqual(normalized[0].Input["limit"], calls[0].Input["limit"]) {
				t.Fatal("source or sibling arguments changed")
			}
			if !strings.Contains(strings.Join(normalized[0].RecoverySteps, ","), "schema_normalization") {
				t.Fatal("missing local recovery evidence")
			}
		}
	}
}

func TestRepeatedCDATANumberRecoveryRejectsAmbiguousValues(t *testing.T) {
	tools := cdataNumberTools(map[string]any{"type": "number"})
	for _, raw := range []string{
		"120</![CDATA[121", "120</![CDATA[120.0", "120</![CDATA[", "</![CDATA[120", "120</![CDATA[120</![CDATA[120",
		"120x</![CDATA[120x", "01</![CDATA[01", "NaN</![CDATA[NaN", "+1</![CDATA[+1", "true</![CDATA[true", "null</![CDATA[null",
		`"120"</![CDATA["120"`, "120</![CDATA[120 trailing", "print(120</![CDATA[120)",
	} {
		calls := []ParsedToolCall{{Name: "read", Input: map[string]any{"offset": raw}}}
		normalized := NormalizeParsedToolCallsForSchemas(calls, tools)
		if normalized[0].Input["offset"] != raw || ValidateToolArguments(normalized, tools) == nil {
			t.Fatalf("ambiguous value accepted: %q => %#v", raw, normalized)
		}
	}
}

func TestRepeatedCDATANumberRecoveryRespectsSchemaAndLiterals(t *testing.T) {
	const raw = "120</![CDATA[120"
	for _, schema := range []any{
		map[string]any{"type": "string"}, map[string]any{"type": []any{"number", "string"}},
		map[string]any{"anyOf": []any{map[string]any{"type": "number"}, map[string]any{"type": "string"}}},
		map[string]any{}, map[string]any{"type": "integer", "maximum": 100}, map[string]any{"type": "number", "const": 121},
	} {
		calls := []ParsedToolCall{{Name: "read", Input: map[string]any{"offset": raw}}}
		got := NormalizeParsedToolCallsForSchemas(calls, cdataNumberTools(schema))
		if got[0].Input["offset"] != raw {
			t.Fatalf("schema/literal constraint bypassed: %v => %#v", schema, got)
		}
	}
	for _, schema := range []any{
		map[string]any{"$ref": "#/$defs/index"},
		map[string]any{"allOf": []any{map[string]any{"type": "integer"}, map[string]any{"minimum": 100, "maximum": 200}}},
	} {
		tools := cdataNumberTools(schema)
		tools[0].(map[string]any)["parameters"].(map[string]any)["$defs"] = map[string]any{"index": map[string]any{"type": "integer"}}
		got := NormalizeParsedToolCallsForSchemas([]ParsedToolCall{{Name: "read", Input: map[string]any{"offset": raw}}}, tools)
		if got[0].Input["offset"] != float64(120) || ValidateToolArguments(got, tools) != nil {
			t.Fatalf("compiled schema recovery failed: %#v", got)
		}
	}
	for _, tools := range []any{nil, []any{map[string]any{"name": "read"}}} {
		calls := []ParsedToolCall{{Name: "read", Input: map[string]any{"offset": raw}}}
		if got := NormalizeParsedToolCallsForSchemas(calls, tools); got[0].Input["offset"] != raw {
			t.Fatal("schema-less value changed")
		}
	}
}
