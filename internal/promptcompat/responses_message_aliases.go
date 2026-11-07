package promptcompat

import (
	"fmt"
	"strings"
)

// Standalone text items use the content-block validator. Message-level text
// is a compatibility alias for content, not a second body to discard.
func validateResponsesMessageTextAliases(item map[string]any, kind string) error {
	if kind == "text" || kind == "input_text" || kind == "output_text" {
		return nil
	}
	text, hasText := item["text"]
	content, hasContent := item["content"]
	if !hasText || !hasContent {
		return nil
	}
	if text == nil && content == nil {
		return nil
	}
	value, ok := text.(string)
	body, textual := responsesMessageBodyText(content)
	if !ok || !textual || value != body {
		return fmt.Errorf("conflicting message text and content aliases")
	}
	return nil
}

// Contextual annotations stay on content; they do not change its text alias.
func responsesMessageBodyText(raw any) (string, bool) {
	if text, ok := raw.(string); ok {
		return text, true
	}
	blocks, ok := raw.([]any)
	if !ok {
		return "", false
	}
	var body strings.Builder
	for _, raw := range blocks {
		if text, ok := raw.(string); ok {
			body.WriteString(text)
			continue
		}
		block, _ := raw.(map[string]any)
		switch strings.ToLower(strings.TrimSpace(asString(block["type"]))) {
		case "text", "input_text", "output_text":
		default:
			return "", false
		}
		text, present := block["text"]
		if !present {
			text = block["content"]
		}
		value, ok := text.(string)
		if !ok {
			return "", false
		}
		body.WriteString(value)
	}
	return body.String(), true
}
