package promptcompat

import (
	"encoding/json"

	"ds2api/internal/prompt"
	"ds2api/internal/toolcall"
)

// Catalog extensions are data attached to a definition, never tool arguments.
// Keep outer and function containers separate, including conflicting optional
// descriptions; only the selected description is consumed by the text heading.
func toolDefinitionContext(tool map[string]any) map[string]any {
	fields := map[string]any{}
	for _, container := range []string{"tool", "function"} {
		object := tool
		if container == "function" {
			object, _ = tool["function"].(map[string]any)
		}
		extra := map[string]any{}
		for key, value := range object {
			if toolDefinitionFieldKind(tool, key, container == "function") != "tool_context" {
				continue
			}
			extra[key] = value
		}
		if len(extra) > 0 {
			fields[container] = extra
		}
	}
	return fields
}

func toolDefinitionFieldKind(tool map[string]any, key string, nested bool) string {
	switch key {
	case "name":
		return "tool_name"
	case "description":
		object := tool
		if nested {
			object, _ = tool["function"].(map[string]any)
		}
		_, selected, _ := toolcall.ExtractToolMeta(tool)
		if text, ok := object[key].(string); !ok || text != selected {
			return "tool_context"
		}
		return "tool_description"
	case "parameters", "input_schema", "inputSchema", "schema", "parametersJsonSchema", "parameters_json_schema":
		if tool["type"] == "custom" {
			return "tool_context" // Custom execution consumes raw input, not this schema.
		}
		return "tool_schema"
	case "input_examples":
		return "tool_examples"
	case "cache_control":
		return "transport_only"
	case "type", "function":
		if !nested {
			return "tool_definition_container"
		}
	}
	return "tool_context"
}

func toolDefinitionContextText(tool map[string]any) string {
	fields := toolDefinitionContext(tool)
	if len(fields) == 0 {
		return ""
	}
	data, err := json.Marshal(prompt.SanitizeContextValue(fields))
	if err != nil {
		return ""
	}
	return "\nTool context (JSON data): " + string(data)
}
