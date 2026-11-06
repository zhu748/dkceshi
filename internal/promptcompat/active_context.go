package promptcompat

import "strings"

// SplitActiveContext keeps the newest user task and its entire tool loop live.
// Earlier instruction messages stay in history at their original position and
// are repeated in the live context, in order, as the effective instruction set.
// A leading system prompt alone is not history worth uploading.
func SplitActiveContext(messages []any) (history, live []any) {
	messages = NormalizeLegacyToolHistory(messages)
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
	dependencies, err := toolHistoryDependencies(messages)
	if err != nil {
		return nil, messages
	}
	for i := len(messages) - 1; i >= cut; i-- {
		if callIndex, ok := dependencies[i]; ok && callIndex < cut {
			cut = callIndex
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
