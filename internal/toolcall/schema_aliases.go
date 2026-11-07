package toolcall

import (
	"encoding/json"
	"fmt"
)

// Every accepted schema spelling denotes the same catalog entry. Compare the
// original JSON values (including explicit null), before selecting an alias.
func validateToolSchemaAliases(tool map[string]any) error {
	function, _ := tool["function"].(map[string]any)
	var values [8]any
	count := 0
	for _, object := range []map[string]any{tool, function} {
		for _, key := range []string{"parameters", "input_schema", "inputSchema", "schema"} {
			value, present := object[key]
			if !present {
				continue
			}
			values[count], count = value, count+1
		}
	}
	if count < 2 {
		return nil // Normal catalogs do not need another schema serialization.
	}
	var first []byte
	for _, value := range values[:count] {
		data, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("tool schema cannot be encoded: %w", err)
		}
		if first != nil && string(first) != string(data) {
			return fmt.Errorf("conflicting tool schema aliases")
		}
		first = data
	}
	return nil
}
