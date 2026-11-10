package promptcompat

import "strings"

// Scan only source message containers; normalizing the entire request here
// would duplicate argument decoding and attachment copies before auth.
func responsesSystemMessagesMentionTools(req map[string]any) bool {
	if text, ok := req["instructions"].(string); ok && textMentionsTools(text) {
		return true
	}
	if messages, ok := req["messages"].([]any); ok && len(messages) > 0 {
		return MessagesDeclareToolsInSystemText(messages)
	}
	var items []any
	switch input := req["input"].(type) {
	case []any:
		items = input
	case map[string]any:
		items = []any{input}
	}
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok || !responsesMessageContext(item, strings.ToLower(strings.TrimSpace(asString(item["type"])))) {
			continue
		}
		body, _ := responsesMessageBody(item)
		if MessagesDeclareToolsInSystemText([]any{map[string]any{"role": item["role"], "content": body}}) {
			return true
		}
	}
	return false
}
