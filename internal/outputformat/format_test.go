package outputformat

import "testing"

func TestJSONOutputValidationIsStrictAndLossless(t *testing.T) {
	for _, tc := range []struct {
		format Format
		text   string
		valid  bool
	}{
		{Format{Type: "json"}, `[1,null,true]`, true},
		{Format{Type: "json_object"}, `[1]`, false},
		{Format{Type: "json_object"}, `{"id":9007199254740993}`, true},
		{Format{Type: "json"}, `{} {}`, false},
		{Format{Type: "json"}, "```json\n{}\n```", false},
		{Format{Type: "json_schema", Schema: map[string]any{"const": "answer"}}, `"answer"`, true},
		{Format{Type: "json_schema", Schema: map[string]any{"const": "answer"}}, `"different"`, false},
	} {
		if got := tc.format.Validate(tc.text); (got == nil) != tc.valid {
			t.Fatalf("format=%+v text=%s err=%v", tc.format, tc.text, got)
		}
	}
}

func TestOutputSchemasCannotLoadExternalResources(t *testing.T) {
	for _, ref := range []string{"https://example.com/schema.json", "file:///etc/passwd"} {
		if (Format{Type: "json_schema", Schema: map[string]any{"$ref": ref}}).Check() == nil {
			t.Fatalf("external schema accepted: %s", ref)
		}
	}
}
