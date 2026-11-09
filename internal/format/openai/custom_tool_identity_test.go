package openai

import (
	"testing"

	"ds2api/internal/promptcompat"
	"ds2api/internal/toolcall"
)

func TestCustomToolRenderingUsesCanonicalName(t *testing.T) {
	tools := []any{map[string]any{"type": "custom", "name": " custom "}}
	standard, err := promptcompat.NormalizeOpenAIResponsesRequest(nil, map[string]any{"model": "deepseek-v4.1-flash", "input": "hello", "tools": tools}, "")
	if err != nil {
		t.Fatal(err)
	}
	calls := []toolcall.ParsedToolCall{{Name: standard.ToolNames[0], Input: map[string]any{"input": "  EXACT\r\n"}}}
	if err := toolcall.ValidateToolArguments(calls, standard.ToolsRaw); err != nil {
		t.Fatal(err)
	}
	items := toResponsesFunctionCallItems(calls, standard.ToolsRaw)
	item := items[0].(map[string]any)
	if item["type"] != "custom_tool_call" || item["input"] != "  EXACT\r\n" || item["name"] != "custom" {
		t.Fatalf("custom tool wire shape changed: %#v", item)
	}
}
