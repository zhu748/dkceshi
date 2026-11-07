package promptcompat

import (
	"ds2api/internal/prompt"
	"fmt"
)

// Native system containers render text blocks only. Unsupported shapes must
// fail before adapters can reduce the caller's instructions to an empty string.
func ValidateSystemTextContainer(raw any, partsKey string) error {
	if raw == nil {
		return nil
	}
	if _, ok := raw.(string); ok {
		return nil
	}
	if partsKey != "" {
		container, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("system instruction must be text or a content object")
		}
		if err := validatePartsTextAliases(container, partsKey); err != nil {
			return err
		}
		if _, hasParts := container[partsKey]; !hasParts {
			if _, ok := container["text"].(string); ok {
				return prompt.ValidateTextAliases(container, "text", "content")
			}
		}
		raw = container[partsKey]
	}
	blocks, ok := raw.([]any)
	if !ok {
		return fmt.Errorf("system instruction requires an array of text blocks")
	}
	for i, raw := range blocks {
		block, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("system block %d must be a text object", i)
		}
		if _, ok := block["text"].(string); !ok {
			return fmt.Errorf("system block %d requires string text", i)
		}
		if err := prompt.ValidateTextAliases(block, "text", "content"); err != nil {
			return fmt.Errorf("system block %d: %w", i, err)
		}
		if kind, present := block["type"]; partsKey == "" && present && kind != "" && kind != "text" {
			return fmt.Errorf("system block %d has an unsupported type", i)
		}
	}
	return nil
}
