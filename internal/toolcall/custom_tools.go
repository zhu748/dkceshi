package toolcall

import "encoding/json"

// Custom tools retain their raw string as a single input parameter in the
// shared textual tool protocol. Only the Responses renderer changes its wire shape.
func IsCustomTool(toolsRaw any, name string) bool {
	tools, _ := toolsRaw.([]any)
	for _, raw := range tools {
		tool, _ := raw.(map[string]any)
		if tool["type"] == "custom" && tool["name"] == name {
			return true
		}
	}
	return false
}

func ToolSchemaForPrompt(tool map[string]any) any {
	name, _, schema := ExtractToolMeta(tool)
	if tool["type"] == "custom" && name != "" {
		description := "Exact raw text input for this custom tool; preserve all bytes and line breaks."
		if format := tool["format"]; format != nil {
			if data, err := json.Marshal(format); err == nil {
				description += " Required input format: " + string(data)
			}
		}
		return map[string]any{"type": "object", "properties": map[string]any{"input": map[string]any{"type": "string", "description": description}}, "required": []any{"input"}, "additionalProperties": false}
	}
	return schema
}
