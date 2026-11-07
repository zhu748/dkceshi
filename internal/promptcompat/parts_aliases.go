package promptcompat

import (
	"fmt"
	"strings"

	"ds2api/internal/prompt"
)

// Native parts and compatibility text spell the same body. Check both before
// an adapter picks one; empty parts cannot erase a nonempty text alias.
func validatePartsTextAliases(container map[string]any, field string) error {
	if err := prompt.ValidateTextAliases(container, "text", "content"); err != nil {
		return err
	}
	parts, present := container[field]
	if !present {
		return nil
	}
	var joined strings.Builder
	if entries, ok := parts.([]any); ok {
		for _, raw := range entries {
			block, _ := raw.(map[string]any)
			text, ok := block["text"].(string)
			if !ok {
				if _, hasText := container["text"]; hasText {
					return fmt.Errorf("text alias cannot replace non-text parts")
				}
				continue
			}
			joined.WriteString(text)
		}
	}
	for _, key := range []string{"text", "content"} {
		if text, exists := container[key]; exists && text != joined.String() {
			return fmt.Errorf("conflicting %s and %s aliases", field, key)
		}
	}
	return nil
}
