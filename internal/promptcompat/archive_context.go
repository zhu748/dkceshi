package promptcompat

import "strings"

// LatestUserInput returns the original prompt-visible text, including whitespace.
func LatestUserInput(messages []any) (string, bool) {
	for i := len(messages) - 1; i >= 0; i-- {
		msg, _ := messages[i].(map[string]any)
		if normalizeOpenAIRoleForPrompt(asString(msg["role"])) != "user" {
			continue
		}
		text := NormalizeOpenAIContentForPrompt(msg["content"])
		if strings.TrimSpace(text) != "" {
			return text, true
		}
	}
	return "", false
}
