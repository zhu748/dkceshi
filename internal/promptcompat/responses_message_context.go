package promptcompat

// Roleless compatibility bodies are user messages and need the same source
// validation as explicitly typed messages before any fallback can pick text.
func responsesMessageContext(item map[string]any, kind string) bool {
	if !responsesMessageItem(kind) {
		return false
	}
	_, content := item["content"]
	_, text := item["text"].(string)
	_, role := item["role"]
	return kind != "" || role || content || text
}

func responsesMessageBody(item map[string]any) (any, bool) {
	if content, present := item["content"]; present {
		return content, true
	}
	if text, ok := item["text"].(string); ok {
		return text, true
	}
	return nil, false
}
