package toolcall

import (
	"fmt"
	"testing"
)

func TestBatchSchemaReuseKeepsExactNamesAndDoesNotLeakAcrossRequests(t *testing.T) {
	tools := make([]any, 16)
	calls := make([]ParsedToolCall, 128)
	for i := range tools {
		typ := "integer"
		if i%2 == 0 {
			typ = "string"
		}
		tools[i] = map[string]any{"name": fmt.Sprintf("tool%d", i), "parameters": map[string]any{"type": "object", "properties": map[string]any{"value": map[string]any{"type": typ}}}}
	}
	for i := range calls {
		calls[i] = ParsedToolCall{Name: fmt.Sprintf("tool%d", i%len(tools)), Input: map[string]any{"value": float64(123)}, StringLiterals: map[string]string{"value": "123"}}
	}
	out := NormalizeParsedToolCallsForSchemas(calls, tools)
	if err := ValidateToolArguments(out, tools); err != nil {
		t.Fatal(err)
	}
	for i, call := range out {
		_, isString := call.Input["value"].(string)
		if isString != (i%2 == 0) || calls[i].Input["value"] != float64(123) {
			t.Fatal("batch cache eviction changed schema ownership or source arguments")
		}
	}
	tools[0].(map[string]any)["parameters"] = map[string]any{"type": "object", "properties": map[string]any{"value": map[string]any{"type": "integer"}}}
	next := NormalizeParsedToolCallsForSchemas(calls[:1], tools)
	if _, isString := next[0].Input["value"].(string); isString {
		t.Fatal("previous request's compiled schema leaked into the next request")
	}
}
