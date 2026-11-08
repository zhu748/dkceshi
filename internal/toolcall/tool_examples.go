package toolcall

import (
	"encoding/json"
	"fmt"
)

// ToolInputExamples accepts flattened and function-wrapped catalogs equally.
// Alias validation runs before prompt assembly, including explicit null values.
func ToolInputExamples(tool map[string]any) (any, bool) {
	if value, present := tool["input_examples"]; present {
		return value, true
	}
	function, _ := tool["function"].(map[string]any)
	value, present := function["input_examples"]
	return value, present
}

func validateToolExampleAliases(tool map[string]any) error {
	function, _ := tool["function"].(map[string]any)
	outer, hasOuter := tool["input_examples"]
	inner, hasInner := function["input_examples"]
	if !hasOuter || !hasInner {
		return nil
	}
	first, err := json.Marshal(outer)
	if err != nil {
		return err
	}
	second, err := json.Marshal(inner)
	if err != nil {
		return err
	}
	if string(first) != string(second) {
		return fmt.Errorf("conflicting tool input_examples aliases")
	}
	return nil
}
