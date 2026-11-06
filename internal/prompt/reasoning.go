package prompt

import "strings"

// JoinReasoningRepresentations combines alternate fields, deduplicating only
// byte-identical representations. Repeated blocks inside one field stay intact.
func JoinReasoningRepresentations(texts ...string) string {
	var parts []string
	seen := make(map[string]bool, len(texts))
	for _, text := range texts {
		if strings.TrimSpace(text) != "" && !seen[text] {
			parts = append(parts, text)
			seen[text] = true
		}
	}
	return strings.Join(parts, "\n")
}

func AssistantReasoningText(msg map[string]any) string {
	return JoinReasoningRepresentations(ReasoningText(msg["reasoning_content"]), ContentReasoningText(msg["content"]))
}

func ContentReasoningText(value any) string {
	if _, ordinaryText := value.(string); ordinaryText {
		return ""
	}
	return ReasoningText(value)
}

func ReasoningText(value any) string {
	switch value := value.(type) {
	case string:
		return value
	case []any:
		var parts []string
		for _, item := range value {
			if block, ok := item.(map[string]any); ok {
				if text := reasoningBlockText(block); text != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		return reasoningBlockText(value)
	}
	return ""
}

func reasoningBlockText(block map[string]any) string {
	kind, _ := block["type"].(string)
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "reasoning", "thinking":
		for _, key := range []string{"text", "thinking", "content"} {
			if text, ok := block[key].(string); ok && strings.TrimSpace(text) != "" {
				return text
			}
		}
	}
	return ""
}
