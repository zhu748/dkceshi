package promptcompat

import (
	"strings"
	"testing"
)

func TestToolCatalogRecognitionAndDeliveryAgree(t *testing.T) {
	for _, tc := range []struct {
		text string
		want bool
	}{
		{"Do not call tools. Answer in plain text.", false},
		{"Explain mathematical functions and tooltips.", false},
		{"You are Claude Code, Anthropic's official CLI tool.", false},
		{"Available tools: none", false},
		{"Available tools: read_file(path string)", true},
		{"# Tools\n- read_file(path: string)", true},
		{"# Tools\n- `read_file(path: string)`", true},
		{"# Tools\n### read_file\nParameters: {}", true},
		{"Available tools:\n1. **read_file**(path: string)", true},
		{"Available tools:\n+ `read_file`(path: string)", true},
		{"Explain `read_file` without calling tools.", false},
		{"# Tools\n- `none`", false},
		{"# Tools\n- `read_file` — Read a file", true},
		{"### **Tool: read_file**\nParameters: {}", true},
		{"You have access to these tools:\n\nread_file(path)", true},
		{"Tool: read_file\nDescription: Read a file\nParameters: {}", true},
		{"可用工具：read_file", true},
	} {
		t.Run(tc.text, func(t *testing.T) {
			req := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{map[string]any{"role": "developer", "content": tc.text}, map[string]any{"role": "user", "content": "hello"}}}
			std, err := NormalizeOpenAIChatRequest(nil, req, "")
			if err != nil {
				t.Fatal(err)
			}
			for name, got := range map[string]bool{"route": RequestBodyHasTools(req), "prompt": strings.Contains(std.FinalPrompt, "工具调用格式规范"), "history split": std.SystemTextToolsApply()} {
				if got != tc.want {
					t.Fatalf("%s=%v want=%v", name, got, tc.want)
				}
			}
			if !strings.Contains(std.FinalPrompt, tc.text) {
				t.Fatal("caller instruction changed")
			}
		})
	}
}

func TestExplicitEmptyCatalogCannotReactivateTextTools(t *testing.T) {
	req := map[string]any{"model": "deepseek-v4.1-flash", "tools": []any{}, "messages": []any{map[string]any{"role": "system", "content": "Available tools: read_file"}, map[string]any{"role": "user", "content": "hello"}}}
	std, err := NormalizeOpenAIChatRequest(nil, req, "")
	if err != nil {
		t.Fatal(err)
	}
	if RequestBodyHasTools(req) || std.SystemTextToolsApply() || strings.Contains(std.FinalPrompt, "工具调用格式规范") || !std.ToolChoice.IsNone() {
		t.Fatal("empty catalog reactivated tools")
	}
}

func TestResponsesPendingReasoningSurvivesMessageReasoning(t *testing.T) {
	got := NormalizeResponsesInputAsMessages([]any{
		map[string]any{"type": "reasoning", "summary": []any{map[string]any{"text": "first reason"}}},
		map[string]any{"role": "assistant", "reasoning_content": "second reason", "tool_calls": []any{map[string]any{"id": "c1", "function": map[string]any{"name": "read", "arguments": "{}"}}}},
	})
	text := RenderPromptText(got, false)
	if !strings.Contains(text, "first reason\nsecond reason") {
		t.Fatalf("reasoning lost/reordered: %s", text)
	}
}
