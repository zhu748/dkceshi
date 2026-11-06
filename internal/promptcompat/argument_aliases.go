package promptcompat

import (
	"encoding/json"
	"fmt"

	"ds2api/internal/jsonvalue"
)

// Compatibility shapes may name the same arguments twice. Compare canonical
// JSON representations, preserving number precision and treating undecodable
// historical strings as opaque text. Never silently prefer conflicting data.
func validateToolArgumentAliases(call, function map[string]any) error {
	var values [4]any
	count := 0
	for _, object := range []map[string]any{call, function} {
		for _, field := range []string{"arguments", "input"} {
			if value, present := object[field]; present {
				values[count] = value
				count++
			}
		}
	}
	if count < 2 {
		return nil
	}
	var first string
	seen := false
	for _, value := range values[:count] {
		if text, ok := value.(string); ok {
			var parsed any
			if jsonvalue.Unmarshal([]byte(text), &parsed) == nil {
				value = parsed
			}
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("tool arguments cannot be encoded: %w", err)
		}
		if seen && first != string(encoded) {
			return fmt.Errorf("conflicting tool argument aliases")
		}
		first, seen = string(encoded), true
	}
	return nil
}
