package toolcall

import (
	"context"
	"reflect"
	"testing"

	"ds2api/internal/jsonvalue"
)

func TestRepairCannotChangeParameterTypeByAddingCDATA(t *testing.T) {
	for _, raw := range []string{`true`, `null`, `9007199254740993`, `{"key":"value"}`, `"quoted string"`} {
		source := `<|EPSE|call name="lookup"><|EPSE|parameter name="value">` + raw + `</|EPSE|parameter></|EPSE|call>`
		output := `<tool_calls><invoke name="lookup"><parameter name="value"><![CDATA[` + raw + `]]></parameter></invoke></tool_calls>`
		calls, ok := RepairToolCallsWithLLM(context.Background(), source, func(context.Context, string) (string, error) { return output, nil })
		if ok || len(calls) != 0 {
			t.Fatalf("repair changed source type for %s: %#v", raw, NormalizeParsedToolCallsForSchemas(calls, nil))
		}
	}
}

func TestRepairPreservesUnwrappedTypedParameters(t *testing.T) {
	for _, raw := range []string{`true`, `null`, `9007199254740993`, `{"key":"value"}`, `"quoted string"`} {
		source := `<|EPSE|call name="lookup"><|EPSE|parameter name="value">` + raw + `</|EPSE|parameter></|EPSE|call>`
		output := `<tool_calls><invoke name="lookup"><parameter name="value">` + raw + `</parameter></invoke></tool_calls>`
		calls, ok := RepairToolCallsWithLLM(context.Background(), source, func(context.Context, string) (string, error) { return output, nil })
		var expected any
		if err := jsonvalue.Unmarshal([]byte(raw), &expected); err != nil {
			t.Fatal(err)
		}
		calls = NormalizeParsedToolCallsForSchemas(calls, nil)
		if !ok || len(calls) != 1 || !reflect.DeepEqual(calls[0].Input["value"], expected) {
			t.Fatalf("format-only repair changed typed parameter %s: %#v", raw, calls)
		}
	}
}
