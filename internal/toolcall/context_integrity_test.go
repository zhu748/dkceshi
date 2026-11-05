package toolcall

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestContextIntegritySharedParserCases(t *testing.T) {
	data, err := os.ReadFile("../../tests/fixtures/tool-context-fidelity.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name, Parameter, Body string
		Expected              any
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			raw := `<tool_calls><invoke name="write"><parameter name="` + tc.Parameter + `">` + tc.Body + `</parameter></invoke></tool_calls>`
			parsed := ParseToolCallsDetailed(raw, nil)
			if len(parsed.Calls) != 1 || parsed.SawToolCallIntent {
				t.Fatalf("call not completely parsed: %+v", parsed)
			}
			if got := parsed.Calls[0].Input[tc.Parameter]; !reflect.DeepEqual(got, tc.Expected) {
				t.Fatalf("argument changed: got %#v, want %#v", got, tc.Expected)
			}
		})
	}
}

func TestLooseJSONRepairPreservesAllStringTokens(t *testing.T) {
	for _, literal := range []string{`{bare: x}`, `prefix: {first}, {second}`, `quote " and slash \`, "中文\n\t{key: value}", `: {"nested":1}, {"nested":2}`} {
		encoded, _ := json.Marshal(literal)
		raw := `[{note: ` + string(encoded) + `, values: {"x":1}, {"x":2}}]`
		var result []struct {
			Note   string
			Values []map[string]any
		}
		if err := json.Unmarshal([]byte(RepairLooseJSON(raw)), &result); err != nil {
			t.Fatal(err)
		}
		if len(result) != 1 || result[0].Note != literal || len(result[0].Values) != 2 {
			t.Fatalf("repair changed literal %q: %+v", literal, result)
		}
	}
}
