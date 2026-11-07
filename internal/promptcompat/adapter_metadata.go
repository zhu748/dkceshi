package promptcompat

import (
	"strings"

	"ds2api/internal/prompt"
)

// PreserveAdapterMessageMetadata assigns source message identity to the first
// matching fragment and prefill to the last. Tool-result identities generated
// by the adapter remain authoritative. Fragment ordering never changes.
func PreserveAdapterMessageMetadata(messages []any, source map[string]any, role string) []any {
	first, last := -1, -1
	for i, raw := range messages {
		message, _ := raw.(map[string]any)
		if message["role"] == role {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 {
		if !hasPromptMessageMetadata(source) && (role != "assistant" || prompt.ReasoningText(source["reasoning_content"]) == "") {
			return messages
		}
		message := map[string]any{"role": role, "content": ""}
		copyPromptMessageMetadata(message, source)
		messages = append([]any{message}, messages...)
		first, last = 0, 0
	}
	for _, key := range []string{"id", "prefix", "name", "tool_call_id", "is_error"} {
		value, present := source[key]
		if !present {
			continue
		}
		for i := first; i <= last; i++ {
			message, _ := messages[i].(map[string]any)
			if message["role"] != role || key == "id" && i != first || key == "prefix" && i != last {
				continue
			}
			if _, exists := message[key]; !exists {
				message[key] = value
			}
		}
	}
	if role == "assistant" {
		preserveAdapterAssistantContext(messages, source, first, last)
	}
	return messages
}

func preserveAdapterAssistantContext(messages []any, source map[string]any, first, last int) {
	var parts []string
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		if message["role"] == "assistant" {
			if text := prompt.ReasoningText(message["reasoning_content"]); text != "" {
				parts = append(parts, text)
			}
		}
	}
	if text := prompt.ReasoningText(source["reasoning_content"]); text != "" && text != strings.Join(parts, "\n") {
		message := messages[first].(map[string]any)
		if existing := prompt.ReasoningText(message["reasoning_content"]); existing != "" {
			text += "\n" + existing
		}
		message["reasoning_content"] = text
	}
	message := messages[last].(map[string]any)
	for _, key := range []string{"refusal", "audio"} {
		if value, present := source[key]; present {
			message[key] = value
		}
	}
}
