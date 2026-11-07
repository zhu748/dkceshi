package promptcompat

import (
	"ds2api/internal/prompt"
	"fmt"
	"strings"
)

// ValidateContentContainers checks native message/block containers before an
// adapter can drop malformed entries. Unknown object blocks remain compatible.
func ValidateContentContainers(messages []any, field string, allowString bool) error {
	for i, raw := range messages {
		message, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("messages[%d] must be an object", i)
		}
		if err := ValidateMessageMetadata(message); err != nil {
			return fmt.Errorf("messages[%d]: %w", i, err)
		}
		role := strings.ToLower(strings.TrimSpace(asString(message["role"])))
		if role == "assistant" || field == "parts" && role == "model" {
			if err := validateAssistantContextFields(message); err != nil {
				return fmt.Errorf("messages[%d]: %w", i, err)
			}
		}
		if field == "parts" {
			if err := validatePartsTextAliases(message, field); err != nil {
				return fmt.Errorf("messages[%d]: %w", i, err)
			}
		}
		content, present := message[field]
		if !present && field == "parts" {
			if _, compatible := message["text"].(string); compatible {
				continue
			}
		}
		if allowString {
			if _, ok := content.(string); ok {
				continue
			}
		}
		parts, ok := content.([]any)
		if !ok {
			return fmt.Errorf("messages[%d].%s must be a content array", i, field)
		}
		for j, raw := range parts {
			block, ok := raw.(map[string]any)
			if !ok {
				return fmt.Errorf("messages[%d].%s[%d] must be a content object", i, field, j)
			}
			if err := validateKnownContentFields(block, field == "parts"); err != nil {
				return fmt.Errorf("messages[%d].%s[%d]: %w", i, field, j, err)
			}
		}
	}
	return nil
}

func validateKnownContentFields(block map[string]any, nativePart bool) error {
	kind := strings.ToLower(strings.TrimSpace(asString(block["type"])))
	if kind == "text" || nativePart && block["text"] != nil {
		if err := prompt.ValidateTextAliases(block, "text", "content"); err != nil {
			return err
		}
	}
	if nativePart || kind == "text" {
		if value, present := block["text"]; present {
			if _, ok := value.(string); !ok {
				return fmt.Errorf("text must be a string")
			}
		} else if kind == "text" {
			return fmt.Errorf("text block requires text")
		}
	}
	if kind == "thinking" {
		if err := prompt.ValidateTextAliases(block, "thinking", "text", "content"); err != nil {
			return err
		}
		hasText := false
		for _, field := range []string{"thinking", "text", "content"} {
			if value, present := block[field]; present {
				if _, ok := value.(string); !ok {
					return fmt.Errorf("thinking field %s must be a string", field)
				}
				hasText = true
			}
		}
		if !hasText {
			return fmt.Errorf("thinking block requires text")
		}
	}
	if err := validateNativeToolContext(block, nativePart); err != nil {
		return err
	}
	if !nativePart {
		return nil
	}
	if value, present := block["thought"]; present {
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("thought must be a boolean")
		}
	}
	payloads := 0
	for _, field := range []string{"text", "functionCall", "functionResponse", "inlineData", "inline_data", "fileData", "file_data", "executableCode", "codeExecutionResult"} {
		if _, present := block[field]; present {
			payloads++
		}
	}
	if payloads > 1 {
		return fmt.Errorf("part must contain one payload; multiple payload fields would discard context")
	}
	for _, field := range []string{"functionCall", "functionResponse"} {
		if value, present := block[field]; present {
			if _, ok := value.(map[string]any); !ok {
				return fmt.Errorf("%s must be an object", field)
			}
		}
	}
	return nil
}
