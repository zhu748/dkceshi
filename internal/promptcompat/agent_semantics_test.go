package promptcompat

import "testing"

func TestRegressionReasoningOutputRoundTrip(t *testing.T) {
	input := []any{
		map[string]any{"type": "reasoning", "summary": []any{map[string]any{"type": "summary_text", "text": "must inspect file"}}, "content": []any{map[string]any{"type": "reasoning_text", "text": "must inspect file"}}},
		map[string]any{"type": "function_call", "call_id": "call_1", "name": "read_file", "arguments": `{"path":"a.go"}`},
	}
	got := NormalizeResponsesInputAsMessages(input)
	for _, raw := range got {
		if raw.(map[string]any)["role"] == "user" {
			t.Fatalf("assistant reasoning became a user message: %#v", got)
		}
	}
}
func TestRegressionOfficialAllowedToolsShape(t *testing.T) {
	tools := []any{map[string]any{"type": "function", "name": "lookup", "parameters": map[string]any{"type": "object"}}}
	_, err := parseToolChoicePolicy(map[string]any{"type": "allowed_tools", "mode": "auto", "tools": []any{map[string]any{"type": "function", "name": "lookup"}}}, tools)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRegressionCustomToolRoundTrip(t *testing.T) {
	items := []any{
		map[string]any{"type": "custom_tool_call", "call_id": "call_patch", "name": "apply_patch", "input": "*** Begin Patch\n*** End Patch"},
		map[string]any{"type": "custom_tool_call_output", "call_id": "call_patch", "output": "success"},
	}
	got := NormalizeResponsesInputAsMessages(items)
	if len(got) != 2 || got[1].(map[string]any)["role"] != "tool" {
		t.Fatalf("custom call/result lost their semantic roles: %#v", got)
	}
}
