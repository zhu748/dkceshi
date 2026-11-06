package promptcompat

import (
	"fmt"
	"strings"
)

func validateAssistantContextFields(message map[string]any) error {
	if err := validateAssistantReasoningField(message["reasoning_content"]); err != nil {
		return err
	}
	if value := message["refusal"]; value != nil {
		if _, ok := value.(string); !ok {
			return fmt.Errorf("refusal must be a string")
		}
	}
	if value := message["audio"]; value != nil {
		audio, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("audio must be an object")
		}
		for _, field := range []string{"id", "transcript"} {
			if value := audio[field]; value != nil {
				if _, ok := value.(string); !ok {
					return fmt.Errorf("audio.%s must be a string", field)
				}
			}
		}
	}
	return nil
}

func validateAssistantReasoningField(value any) error {
	var blocks []any
	switch value := value.(type) {
	case nil, string:
		return nil
	case []any:
		blocks = value
	case map[string]any:
		blocks = []any{value}
	default:
		return fmt.Errorf("reasoning_content must be text or reasoning blocks")
	}
	for _, raw := range blocks {
		block, ok := raw.(map[string]any)
		kind := strings.ToLower(strings.TrimSpace(asString(block["type"])))
		if !ok || kind != "thinking" && kind != "reasoning" {
			return fmt.Errorf("reasoning_content requires thinking/reasoning blocks")
		}
	}
	return validatePromptMessageContent(blocks)
}
