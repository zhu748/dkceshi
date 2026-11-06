package promptcompat

import "testing"

func TestNativeToolAliasesFailBeforeFlattening(t *testing.T) {
	for _, tc := range []struct {
		part  bool
		block map[string]any
	}{
		{false, map[string]any{"type": "tool_use", "name": "read", "id": "a", "tool_use_id": "b"}},
		{false, map[string]any{"type": "tool_use", "name": "read", "input": nil}},
		{false, map[string]any{"type": "tool_result", "tool_use_id": "a", "tool_call_id": "b", "content": "RESULT"}},
		{false, map[string]any{"type": "tool_result", "is_error": "false", "content": "RESULT"}},
		{true, map[string]any{"functionCall": map[string]any{"name": "read", "id": "a", "callId": "b"}}},
		{true, map[string]any{"functionCall": map[string]any{"name": "read", "args": nil}}},
		{true, map[string]any{"functionResponse": map[string]any{"id": "a", "tool_call_id": 42}}},
		{true, map[string]any{"functionResponse": map[string]any{"response": map[string]any{"text": "ONE"}, "output": map[string]any{"text": "TWO"}}}},
		{true, map[string]any{"functionResponse": map[string]any{"response": nil, "output": "TWO"}}},
		{true, map[string]any{"functionResponse": map[string]any{"response": "ONE", "parts": "LOST"}}},
		{true, map[string]any{"functionResponse": map[string]any{"parts": []any{map[string]any{"text": "ONE", "content": "TWO"}}}}},
		{true, map[string]any{"functionCall": map[string]any{"name": "read", "id": "a", "call_id": " ", "callId": "b"}}},
		{true, map[string]any{"text": "ONE", "content": "TWO"}},
	} {
		if err := validateKnownContentFields(tc.block, tc.part); err == nil {
			t.Fatalf("native context would be silently overwritten: %#v", tc.block)
		}
	}
}

func TestNativeToolAliasesAllowMatchingIDsAndResponseObjects(t *testing.T) {
	for _, block := range []map[string]any{
		{"functionCall": map[string]any{"name": "read", "id": "a", "callId": "a", "args": map[string]any{}}},
		{"functionResponse": map[string]any{"response": map[string]any{"a": 1, "b": 2}, "output": map[string]any{"b": 2, "a": 1}}},
		{"text": "EXACT", "content": "EXACT"},
	} {
		if err := validateKnownContentFields(block, true); err != nil {
			t.Fatal(err)
		}
	}
}
