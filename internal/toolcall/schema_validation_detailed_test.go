package toolcall

import "testing"

func TestDetailedValidationKeepsSiblingResultsAndFirstError(t *testing.T) {
	tools := []any{map[string]any{"type": "function", "name": "read", "parameters": map[string]any{"type": "object", "required": []any{"path"}, "properties": map[string]any{"path": map[string]any{"type": "string"}}}}}
	calls := []ParsedToolCall{{Name: "read", Input: map[string]any{}}, {Name: "read", Input: map[string]any{"path": "file.txt"}}}
	results, err := ValidateToolArgumentsDetailed(calls, tools)
	legacy := ValidateToolArguments(calls, tools)
	if err == nil || legacy == nil || err.Error() != legacy.Error() || len(results) != 2 {
		t.Fatalf("batch behavior changed: %+v %v", results, err)
	}
	if !results[0].Checked || !results[0].HasSchema || results[0].Error == nil || !results[1].Checked || results[1].Error != nil {
		t.Fatalf("bad per-call outcomes: %+v", results)
	}
}
func TestDetailedValidationDistinguishesSchemaLessAndUnknown(t *testing.T) {
	tools := []any{map[string]any{"type": "function", "name": "read"}, map[string]any{"type": "custom", "name": "script"}}
	calls := []ParsedToolCall{{Name: "read", Input: map[string]any{}}, {Name: "script", Input: map[string]any{"input": "print(1)"}}, {Name: "unknown", Input: map[string]any{}}}
	results, err := ValidateToolArgumentsDetailed(calls, tools)
	if err == nil || len(results) != 3 {
		t.Fatal("unknown call accepted")
	}
	for _, v := range results[:2] {
		if !v.Checked || v.HasSchema || v.Error != nil {
			t.Fatalf("invalid schema-less result: %+v", v)
		}
	}
	if results[2].Error == nil {
		t.Fatal("missing unknown call error")
	}
}
