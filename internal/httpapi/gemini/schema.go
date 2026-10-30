package gemini

import "strings"

// Gemini's Schema uses uppercase type enum values and nullable; convert those
// adapter-specific fields before the shared JSON Schema compiler sees them.
func normalizeGeminiSchema(raw any) any {
	schema, ok := raw.(map[string]any)
	if !ok {
		return raw
	}
	out := make(map[string]any, len(schema))
	for key, value := range schema {
		switch key {
		case "type":
			if typ, ok := value.(string); ok {
				value = strings.ToLower(typ)
			}
		case "properties", "$defs", "definitions":
			if fields, ok := value.(map[string]any); ok {
				next := make(map[string]any, len(fields))
				for name, child := range fields {
					next[name] = normalizeGeminiSchema(child)
				}
				value = next
			}
		case "items", "additionalProperties", "not", "contains", "if", "then", "else":
			value = normalizeGeminiSchema(value)
		case "anyOf", "oneOf", "allOf", "prefixItems":
			if choices, ok := value.([]any); ok {
				next := make([]any, len(choices))
				for i, child := range choices {
					next[i] = normalizeGeminiSchema(child)
				}
				value = next
			}
		}
		out[key] = value
	}
	if schema["nullable"] == true {
		if typ, ok := out["type"].(string); ok {
			out["type"] = []any{typ, "null"}
		}
	}
	return out
}
