package promptcompat

import "strings"

func hasMessageToolCalls(msg map[string]any) bool {
	calls, _ := msg["tool_calls"].([]any)
	return len(calls) > 0 || msg["function_call"] != nil
}

// SplitActiveContext keeps the newest user task and its entire tool loop live.
// Earlier instruction messages stay in history at their original position and
// are repeated in the live context, in order, as the effective instruction set.
// A leading system prompt alone is not history worth uploading.
func SplitActiveContext(messages []any) (history, live []any) {
	cut := -1
	for i := len(messages) - 1; i >= 0; i-- {
		m, ok := messages[i].(map[string]any)
		if ok && strings.EqualFold(normalizeOpenAIRoleForPrompt(asString(m["role"])), "user") {
			cut = i
			break
		}
	}
	if cut < 0 {
		return nil, messages
	}
	for cut > 0 {
		m, _ := messages[cut-1].(map[string]any)
		if !strings.EqualFold(normalizeOpenAIRoleForPrompt(asString(m["role"])), "user") {
			break
		}
		cut--
	}
	// An active tool result may reference a call before the newest user message.
	// Move the boundary back until all such dependencies remain together.
	callIndex := map[string]int{}
	for i, raw := range messages {
		m, _ := raw.(map[string]any)
		calls, _ := m["tool_calls"].([]any)
		for _, rawCall := range calls {
			c, _ := rawCall.(map[string]any)
			if id := asString(c["id"]); id != "" {
				callIndex[id] = i
			}
		}
	}
	for {
		start := cut
		for _, raw := range messages[cut:] {
			m, _ := raw.(map[string]any)
			if i, ok := callIndex[asString(m["tool_call_id"])]; ok && i < cut {
				cut = i
			}
		}
		if start == cut {
			break
		}
	}
	hasHistory := false
	for _, raw := range messages[:cut] {
		m, _ := raw.(map[string]any)
		role := normalizeOpenAIRoleForPrompt(asString(m["role"]))
		if role != "system" && role != "developer" && len(NormalizeOpenAIMessagesForPrompt([]any{raw}, "")) > 0 {
			hasHistory = true
			break
		}
	}
	if !hasHistory {
		return nil, messages
	}
	for _, raw := range messages[:cut] {
		m, _ := raw.(map[string]any)
		role := normalizeOpenAIRoleForPrompt(asString(m["role"]))
		if role == "system" || role == "developer" {
			live = append(live, raw)
		}
	}
	live = append(live, messages[cut:]...)
	return messages[:cut], live
}
