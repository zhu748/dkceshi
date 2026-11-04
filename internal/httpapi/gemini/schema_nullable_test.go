package gemini

import (
	"ds2api/internal/toolcall"
	"encoding/json"
	"testing"
)

func TestGeminiNullableAppliesToEnumsAndComposedSchemas(t *testing.T) {
	for _, schema := range []map[string]any{
		{"type": "STRING", "enum": []any{"x", "y"}, "nullable": true},
		{"anyOf": []any{map[string]any{"type": "STRING"}, map[string]any{"type": "INTEGER"}}, "nullable": true},
	} {
		compiled, err := toolcall.CompileLocalJSONSchema(normalizeGeminiSchema(schema))
		if err != nil {
			t.Fatal(err)
		}
		if err := compiled.Validate(nil); err != nil {
			t.Fatalf("nullable schema rejected null: %v", err)
		}
		if err := compiled.Validate(map[string]any{}); err == nil {
			t.Fatal("nullable conversion widened non-null values")
		}
	}
}

func TestGeminiSDKBoundsAndNumericEnumsCompileWithoutPrecisionLoss(t *testing.T) {
	schema := map[string]any{"type": "ARRAY", "minItems": "1", "maxItems": "2", "items": map[string]any{"type": "INTEGER", "enum": []any{"9007199254740993", "2"}}}
	normalized := normalizeGeminiSchema(schema)
	compiled, err := toolcall.CompileLocalJSONSchema(normalized)
	if err != nil {
		t.Fatalf("SDK schema was not translated: %v", err)
	}
	for _, value := range []any{[]any{json.Number("9007199254740993")}, []any{json.Number("2"), json.Number("2")}} {
		if err := compiled.Validate(value); err != nil {
			t.Fatalf("valid enum rejected: %v", err)
		}
	}
	for _, value := range []any{[]any{}, []any{json.Number("3")}, []any{json.Number("2"), json.Number("2"), json.Number("2")}, []any{json.Number("9007199254740992")}} {
		if err := compiled.Validate(value); err == nil {
			t.Fatalf("constraint or numeric precision lost: %v", value)
		}
	}
	if schema["minItems"] != "1" || schema["items"].(map[string]any)["enum"].([]any)[0] != "9007199254740993" {
		t.Fatal("schema conversion mutated request")
	}
}

func TestNullableWrapperRetainsLocalReferencesAndInvalidBoundsFail(t *testing.T) {
	schema := map[string]any{"$defs": map[string]any{"Text": map[string]any{"type": "STRING"}}, "$ref": "#/$defs/Text", "nullable": true}
	compiled, err := toolcall.CompileLocalJSONSchema(normalizeGeminiSchema(schema))
	if err != nil {
		t.Fatal(err)
	}
	if compiled.Validate(nil) != nil || compiled.Validate("value") != nil || compiled.Validate(42) == nil {
		t.Fatal("nullable conversion changed local-reference semantics")
	}
	if _, err := toolcall.CompileLocalJSONSchema(normalizeGeminiSchema(map[string]any{"type": "ARRAY", "minItems": "invalid"})); err == nil {
		t.Fatal("invalid bound was silently ignored")
	}
}

func TestGeminiEnumDoesNotExpandUnboundedExponents(t *testing.T) {
	for _, raw := range []string{"1e100000000", "1e-100000000", "0e100000000"} {
		values := normalizeGeminiNumericEnum([]any{raw}, "number").([]any)
		if _, converted := values[0].(json.Number); converted {
			t.Fatalf("unsafe exponent normalized to schema number: %s", raw)
		}
	}
}
