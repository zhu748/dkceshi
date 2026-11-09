package outputformat

import (
	"encoding/json"
	"errors"
	"testing"

	"ds2api/internal/toolcall"
)

func TestOutputValidationBoundsRecursiveInstanceExpansion(t *testing.T) {
	property := map[string]any{"properties": map[string]any{"child": map[string]any{"$ref": "#"}}}
	format := Format{Type: "json_schema", Schema: map[string]any{"allOf": []any{property, property}}}
	if err := format.Check(); err != nil {
		t.Fatal(err)
	}
	value := map[string]any{}
	for range 16 {
		value = map[string]any{"child": value}
	}
	data, _ := json.Marshal(value)
	if err := format.Validate(string(data)); !errors.Is(err, toolcall.ErrSchemaEvaluationLimit) {
		t.Fatal("output format bypassed runtime schema limit", err)
	}
}
