package promptcompat

import "strings"

// NormalizeLegacyToolHistory upgrades legacy messages and persists the FIFO
// links used to validate ID-less modern history. Complete IDs are preserved.
// Caller-owned messages/calls are never changed.
func NormalizeLegacyToolHistory(messages []any) []any {
	needsLinking := false
	links := NewToolCallLinker("call_legacy")
	for _, raw := range messages {
		msg, _ := raw.(map[string]any)
		role := strings.ToLower(strings.TrimSpace(asString(msg["role"])))
		resultID := asString(msg["tool_call_id"])
		needsLinking = needsLinking || msg["function_call"] != nil || role == "function" || role == "tool" &&
			(resultID != strings.TrimSpace(resultID) || strings.TrimSpace(resultID) == "" && strings.TrimSpace(asString(msg["name"])) != "")
		links.Reserve(asString(msg["tool_call_id"]))
		links.Reserve(asString(msg["id"]))
		calls, _ := msg["tool_calls"].([]any)
		for _, rawCall := range calls {
			call, _ := rawCall.(map[string]any)
			id := asString(call["id"])
			links.Reserve(id)
			needsLinking = needsLinking || id == "" || id != strings.TrimSpace(id)
		}
		if fn, ok := msg["function_call"].(map[string]any); ok {
			links.Reserve(asString(fn["id"]))
		}
	}
	if !needsLinking {
		return messages
	}
	out := make([]any, 0, len(messages))
	for _, raw := range messages {
		msg, ok := raw.(map[string]any)
		if !ok {
			out = append(out, raw)
			continue
		}
		copy := make(map[string]any, len(msg))
		for key, value := range msg {
			copy[key] = value
		}
		calls, _ := msg["tool_calls"].([]any)
		if fn, ok := msg["function_call"].(map[string]any); ok && len(calls) == 0 {
			id := asString(fn["id"])
			if id == "" {
				id = asString(msg["id"])
			}
			calls = []any{map[string]any{"id": id, "type": "function", "function": fn}}
			delete(copy, "function_call")
		}
		if len(calls) > 0 {
			linked := make([]any, 0, len(calls))
			for _, rawCall := range calls {
				call, ok := rawCall.(map[string]any)
				if !ok {
					linked = append(linked, rawCall)
					continue
				}
				item := make(map[string]any, len(call))
				for key, value := range call {
					item[key] = value
				}
				item["id"] = links.Call(historyToolName(call), asString(call["id"]))
				linked = append(linked, item)
			}
			copy["tool_calls"] = linked
		}
		role := strings.ToLower(strings.TrimSpace(asString(msg["role"])))
		if role == "function" || role == "tool" {
			copy["role"] = "tool"
			copy["tool_call_id"], copy["name"] = links.Result(asString(msg["name"]), asString(msg["tool_call_id"]))
		}
		out = append(out, copy)
	}
	return out
}

func historyToolName(call map[string]any) string {
	fn, _ := call["function"].(map[string]any)
	if name := asString(fn["name"]); name != "" {
		return name
	}
	return asString(call["name"])
}
