package toolcall

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"
)

func FormatOpenAIToolCalls(calls []ParsedToolCall, toolsRaw any) []map[string]any {
	return FormatValidatedOpenAIToolCalls(NormalizeParsedToolCallsForSchemas(calls, toolsRaw))
}

// Shared final validation owns argument normalization. These renderers only
// allocate delivery IDs and serialize the accepted values, without rerunning
// recovery or schema traversal after the executable batch has been approved.
func FormatValidatedOpenAIToolCalls(calls []ParsedToolCall) []map[string]any {
	out := make([]map[string]any, 0, len(calls))
	for _, c := range calls {
		args, _ := json.Marshal(c.Input)
		out = append(out, map[string]any{
			"id":   "call_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
			"type": "function",
			"function": map[string]any{
				"name":      c.Name,
				"arguments": string(args),
			},
		})
	}
	return out
}

func FormatOpenAIStreamToolCalls(calls []ParsedToolCall, toolsRaw any) []map[string]any {
	return FormatValidatedOpenAIStreamToolCalls(NormalizeParsedToolCallsForSchemas(calls, toolsRaw))
}

func FormatValidatedOpenAIStreamToolCalls(calls []ParsedToolCall) []map[string]any {
	out := make([]map[string]any, 0, len(calls))
	for i, c := range calls {
		args, _ := json.Marshal(c.Input)
		out = append(out, map[string]any{
			"index": i,
			"id":    "call_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
			"type":  "function",
			"function": map[string]any{
				"name":      c.Name,
				"arguments": string(args),
			},
		})
	}
	return out
}
