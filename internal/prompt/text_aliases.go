package prompt

import "fmt"

// Alternate spellings of the same text must agree exactly. Trimming or choosing
// the first nonempty value would discard accepted caller content.
func ValidateTextAliases(block map[string]any, fields ...string) error {
	var first string
	seen := false
	for _, field := range fields {
		value, present := block[field]
		if !present {
			continue
		}
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s must be a string", field)
		}
		if seen && text != first {
			return fmt.Errorf("conflicting text aliases: %s", field)
		}
		first, seen = text, true
	}
	return nil
}
