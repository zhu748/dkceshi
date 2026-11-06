package promptcompat

import (
	"fmt"
	"strings"
)

// Validate before normalization can discard malformed executable history.
// Keep legacy/direct call shapes and ID-less compatibility, but never invent a
// tool name or silently remove an unrenderable call beside its result.
func validateHistoryShapes(messages []any) error {
	for i, raw := range messages {
		msg, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("messages[%d] must be a message object", i)
		}
		if legacy := msg["function_call"]; legacy != nil {
			fn, ok := legacy.(map[string]any)
			if !ok || strings.TrimSpace(asString(fn["name"])) == "" {
				return fmt.Errorf("messages[%d].function_call requires a tool name", i)
			}
			if !strings.EqualFold(strings.TrimSpace(asString(msg["role"])), "assistant") {
				return fmt.Errorf("messages[%d].function_call requires assistant role", i)
			}
		}
		if msg["tool_calls"] == nil {
			continue
		}
		calls, ok := msg["tool_calls"].([]any)
		if !ok {
			return fmt.Errorf("messages[%d].tool_calls must be an array", i)
		}
		if len(calls) > 0 && !strings.EqualFold(strings.TrimSpace(asString(msg["role"])), "assistant") {
			return fmt.Errorf("messages[%d].tool_calls requires assistant role", i)
		}
		if len(calls) > 0 && msg["function_call"] != nil {
			return fmt.Errorf("messages[%d] cannot combine function_call and tool_calls", i)
		}
		for j, rawCall := range calls {
			call, ok := rawCall.(map[string]any)
			if !ok || strings.TrimSpace(historyToolName(call)) == "" {
				return fmt.Errorf("messages[%d].tool_calls[%d] requires a tool name", i, j)
			}
			if fn, present := call["function"]; present {
				function, ok := fn.(map[string]any)
				direct, nested := asString(call["name"]), asString(function["name"])
				if !ok || strings.TrimSpace(nested) == "" || direct != "" && direct != nested {
					return fmt.Errorf("messages[%d].tool_calls[%d] has an invalid function name", i, j)
				}
			}
		}
	}
	return nil
}
