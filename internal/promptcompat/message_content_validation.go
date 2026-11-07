package promptcompat

import (
	"ds2api/internal/prompt"
	"fmt"
	"strings"
)

// Compatibility content may contain arbitrary JSON or plain string arrays.
// Validate only recognized blocks which the serializer would otherwise reduce
// to an empty string. Tool argument/business JSON objects stay opaque.
func validatePromptMessageContent(raw any) error {
	parts, ok := raw.([]any)
	if !ok {
		return nil
	}
	for index, part := range parts {
		block, ok := part.(map[string]any)
		if !ok {
			continue
		}
		if err := ValidateMediaContentBlock(block); err != nil {
			return fmt.Errorf("content[%d]: %w", index, err)
		}
		kind := strings.ToLower(strings.TrimSpace(asString(block["type"])))
		var fields []string
		switch kind {
		case "text", "input_text", "output_text":
			fields = []string{"text", "content"}
		case "thinking", "reasoning":
			fields = []string{"text", "thinking", "content"}
		case "refusal":
			fields = []string{"refusal"}
		default:
			continue
		}
		if err := prompt.ValidateTextAliases(block, fields...); err != nil {
			return fmt.Errorf("content[%d]: %w", index, err)
		}
		hasText := false
		for _, field := range fields {
			if _, present := block[field]; present {
				hasText = true
			}
		}
		if !hasText {
			return fmt.Errorf("content[%d] %s block requires text", index, kind)
		}
	}
	return nil
}
