package openai

import (
	"ds2api/internal/toolcall"
	"encoding/json"
)

func ResponsesToolInput(name string, input map[string]any, toolsRaw any) (string, string) {
	if toolcall.IsCustomTool(toolsRaw, name) {
		text, _ := input["input"].(string)
		return "input", text
	}
	data, _ := json.Marshal(input)
	return "arguments", string(data)
}

func BuildResponsesToolItem(name, field, value, itemID, callID, status string) map[string]any {
	kind := "function_call"
	if field == "input" {
		kind = "custom_tool_call"
	}
	return map[string]any{"id": itemID, "type": kind, "call_id": callID, "name": name, field: value, "status": status}
}

// FormatValidatedChatToolCalls keeps arguments JSON inside strings when crossing
// the Go/Node boundary, so JavaScript cannot round large numeric literals.
func FormatValidatedChatToolCalls(calls []toolcall.ParsedToolCall, toolsRaw any) []map[string]any {
	return toolcall.FormatOpenAIStreamToolCalls(calls, toolsRaw)
}
