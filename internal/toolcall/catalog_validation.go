package toolcall

import (
	"fmt"
	"strings"
)

// A malformed explicit catalog must never become schema-less compatibility.
// Direct schemas are used by normalized Claude/Gemini and Responses requests.
func validateToolCatalog(raw any) error {
	if raw == nil {
		return nil
	}
	tools, ok := raw.([]any)
	if !ok {
		return fmt.Errorf("tools must be an array")
	}
	seen := make(map[string]bool, len(tools))
	for i, rawTool := range tools {
		tool, ok := rawTool.(map[string]any)
		if !ok || len(tool) == 0 {
			return fmt.Errorf("tools[%d] must be a tool object", i)
		}
		typ, validType := tool["type"].(string)
		if tool["type"] != nil && !validType || typ != "" && typ != "function" && typ != "custom" {
			return fmt.Errorf("tools[%d] has an unsupported type", i)
		}
		fn, hasFunction := tool["function"]
		if hasFunction {
			function, ok := fn.(map[string]any)
			if !ok || typ == "custom" {
				return fmt.Errorf("tools[%d].function must be a function object", i)
			}
			nested := strings.TrimSpace(asStringValue(function["name"]))
			direct := strings.TrimSpace(asStringValue(tool["name"]))
			if nested == "" || direct != "" && direct != nested {
				return fmt.Errorf("tools[%d] has a missing or conflicting function name", i)
			}
		}
		name, _, _ := ExtractToolMeta(tool)
		if name == "" {
			return fmt.Errorf("tools[%d] requires a non-empty tool name", i)
		}
		if seen[name] {
			return fmt.Errorf("tools contains duplicate tool name %q", name)
		}
		seen[name] = true
	}
	return nil
}
