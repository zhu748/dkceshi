package promptcompat

import (
	"strings"
	"testing"
)

func TestToolCallLinkerMatchesPendingCallsAndAvoidsIDCollisions(t *testing.T) {
	s := NewToolCallLinker("call_test")
	s.Reserve("call_test_1")
	a := s.Call("lookup", "")
	b := s.Call("lookup", "explicit")
	if a == "call_test_1" {
		t.Fatal("synthetic ID collided with reserved ID")
	}
	if id, name := s.Result("", b); id != b || name != "lookup" {
		t.Fatal("explicit out-of-order result lost")
	}
	if id, _ := s.Result("lookup", ""); id != a {
		t.Fatal("ID-less result was not matched to remaining call")
	}
	if id, _ := s.Result("lookup", ""); id == a || id == b {
		t.Fatal("completed call was matched twice")
	}
	c := s.Call("lookup", "c")
	d := s.Call("lookup", "d")
	for _, want := range []string{c, d} {
		if id, _ := s.Result("lookup", ""); id != want {
			t.Fatalf("expected FIFO match %s, got %s", want, id)
		}
	}
}

func TestResponsesToolFailureAndNullSurviveTheNextAgentTurn(t *testing.T) {
	items := []any{
		map[string]any{"type": "function_call", "name": "lookup", "call_id": "failed_call", "arguments": `{}`},
		map[string]any{"type": "function_call_output", "call_id": "failed_call", "output": nil, "is_error": true},
	}
	messages := NormalizeResponsesInputAsMessages(items)
	if len(messages) != 2 {
		t.Fatalf("tool result lost: %#v", messages)
	}
	text := RenderPromptText(messages, false)
	if strings.Count(text, "failed_call") != 2 || !strings.Contains(text, `"is_error":true`) || !strings.Contains(text, "[Tool]:null") {
		t.Fatalf("tool failure changed: %s", text)
	}
}

func TestOpenAIToolPolicyPreservesSequentialAndAllowedConstraints(t *testing.T) {
	tools := []any{map[string]any{"type": "function", "function": map[string]any{"name": "read"}}, map[string]any{"type": "function", "function": map[string]any{"name": "write"}}}
	req := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{map[string]any{"role": "user", "content": "do work"}}, "tools": tools, "parallel_tool_calls": false, "tool_choice": map[string]any{"type": "auto", "allowed_tools": []any{"read"}}}
	for _, normalize := range []func() (StandardRequest, error){
		func() (StandardRequest, error) { return NormalizeOpenAIChatRequest(nil, req, "") },
		func() (StandardRequest, error) { return NormalizeOpenAIResponsesRequest(nil, req, "") },
	} {
		standard, err := normalize()
		if err != nil {
			t.Fatal(err)
		}
		if !standard.ToolChoice.DisableParallel || !standard.ToolChoice.RestrictAllowed || !standard.ToolChoice.Constrained() || standard.ToolChoice.Allows("write") {
			t.Fatalf("constraints lost: %#v", standard.ToolChoice)
		}
	}
	req["parallel_tool_calls"] = "false"
	if _, err := NormalizeOpenAIChatRequest(nil, req, ""); err == nil {
		t.Fatal("string boolean accepted")
	}
}

func TestForcedToolChoiceCannotOverrideExplicitAllowedSubset(t *testing.T) {
	tools := []any{map[string]any{"type": "function", "function": map[string]any{"name": "read"}}, map[string]any{"type": "function", "function": map[string]any{"name": "write"}}}
	for _, typ := range []string{"", "auto", "function"} {
		for _, name := range []string{"read", "write"} {
			choice := map[string]any{"type": typ, "function": map[string]any{"name": name}, "allowed_tools": []any{"read"}}
			policy, err := parseToolChoicePolicy(choice, tools)
			if name == "write" {
				if err == nil || !strings.Contains(err.Error(), "excluded by allowed_tools") {
					t.Fatalf("conflicting subset silently overridden: policy=%#v err=%v", policy, err)
				}
			} else if err != nil || !policy.Allows("read") || policy.Allows("write") {
				t.Fatalf("valid forced subset lost: policy=%#v err=%v", policy, err)
			}
		}
	}
}
