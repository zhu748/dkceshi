package contextfile

import (
	"strings"
	"unicode/utf8"

	"ds2api/internal/prompt"
	"ds2api/internal/promptcompat"
)

// Large active loops move as one ordered transcript, including their user
// task, calls and results. Moving a result alone would break dependencies.
func largeActiveContext(messages []any) bool {
	chars := 0
	for _, message := range promptcompat.NormalizeOpenAIMessagesForPrompt(messages, "") {
		if message["role"] != "system" && message["role"] != "developer" {
			text, _ := message["content"].(string)
			chars += utf8.RuneCountInString(text)
			chars += utf8.RuneCountInString(prompt.MessageContextText(message))
			if chars > maxInlineFirstTaskChars {
				return true
			}
		}
	}
	return false
}

func externalizedActiveContext(messages []any) (history, live []any) {
	history = messages
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		role, _ := message["role"].(string)
		role = strings.ToLower(strings.TrimSpace(role))
		if role == "system" || role == "developer" {
			live = append(live, raw)
		}
	}
	if len(messages) > 0 {
		last, _ := messages[len(messages)-1].(map[string]any)
		role, _ := last["role"].(string)
		if strings.EqualFold(strings.TrimSpace(role), "assistant") && last["prefix"] == true {
			live = append(live, last)
			history = messages[:len(messages)-1]
		}
	}
	return history, live
}
