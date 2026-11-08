package gemini

import (
	"encoding/json"
	"testing"

	"ds2api/internal/toolcall"
)

func TestNullableSchemaPreservesPointersAcrossWrappedResources(t *testing.T) {
	for _, fixture := range []struct{ schema, valid, invalid string }{
		{`{"type":"OBJECT","nullable":true,"properties":{"a":{"type":"STRING"},"b":{"$ref":"#/properties/a"}}}`, `{"a":"x","b":"y"}`, `{"b":42}`},
		{`{"type":"OBJECT","nullable":true,"properties":{"a b":{"type":"STRING"},"b":{"$ref":"#/properties/a%20b"}}}`, `{"b":"y"}`, `{"b":42}`},
		{`{"type":"OBJECT","properties":{"box":{"type":"OBJECT","nullable":true,"properties":{"x":{"type":"STRING"},"y":{"$ref":"#/properties/box/properties/x"}}}}}`, `{"box":{"y":"yes"}}`, `{"box":{"y":42}}`},
		{`{"$id":"https://example.test/root.json","type":"OBJECT","nullable":true,"$defs":{"Doc":{"$id":"sub.json","nullable":true,"type":"OBJECT","properties":{"x":{"type":"STRING"},"y":{"$ref":"#/properties/x"}}}},"properties":{"doc":{"$ref":"sub.json"}}}`, `{"doc":{"y":"yes"}}`, `{"doc":{"y":42}}`},
		{`{"type":"OBJECT","nullable":true,"properties":{"box":{"$anchor":"Box","nullable":true,"type":"STRING"},"copy":{"$ref":"#Box"}}}`, `{"copy":null}`, `{"copy":42}`},
		{`{"type":"OBJECT","nullable":true,"x-private":{"type":"string"},"properties":{"copy":{"$ref":"#/x-private"}}}`, `{"copy":"ok"}`, `{"copy":42}`},
		{`{"$id":"https://example.test/root.json","type":"OBJECT","nullable":true,"$defs":{"Doc":{"$id":"sub.json","nullable":true,"type":"OBJECT","properties":{"x":{"type":"STRING"}}}},"properties":{"copy":{"$ref":"sub.json#/properties/x"}}}`, `{"copy":"yes"}`, `{"copy":42}`},
		{`{"type":"OBJECT","nullable":true,"properties":{"a/b~c":{"type":"STRING"},"copy":{"$ref":"#/properties/a~1b~0c"}}}`, `{"copy":"yes"}`, `{"copy":42}`},
	} {
		t.Run(fixture.schema, func(t *testing.T) {
			var raw, valid, invalid any
			if err := json.Unmarshal([]byte(fixture.schema), &raw); err != nil {
				t.Fatal(err)
			}
			before, _ := json.Marshal(raw)
			normalized := normalizeGeminiSchema(raw)
			compiled, err := toolcall.CompileLocalJSONSchema(normalized)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(fixture.valid), &valid); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(fixture.invalid), &invalid); err != nil {
				t.Fatal(err)
			}
			if err := compiled.Validate(valid); err != nil {
				t.Fatalf("valid value rejected: %v", err)
			}
			if compiled.Validate(invalid) == nil {
				t.Fatal("constraint was widened by pointer rewriting")
			}
			after, _ := json.Marshal(raw)
			if string(before) != string(after) {
				t.Fatal("source schema changed")
			}
		})
	}
}

func TestNullableWrapperDoesNotMakeInvalidPointersResolve(t *testing.T) {
	for _, ref := range []string{"#/anyOf/0", "#/anyOf/1", "#/anyOf/0/properties/x"} {
		schema := map[string]any{"nullable": true, "type": "OBJECT", "properties": map[string]any{"x": map[string]any{"type": "STRING"}, "copy": map[string]any{"$ref": ref}}}
		if _, err := toolcall.CompileLocalJSONSchema(normalizeGeminiSchema(schema)); err == nil {
			t.Fatalf("invalid source pointer became valid: %s", ref)
		}
	}
}
