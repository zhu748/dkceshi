package claude

import (
	"ds2api/internal/promptcompat"
	"fmt"
)

func claudeToolPolicy(req map[string]any) (promptcompat.ToolChoicePolicy, error) {
	if promptcompat.ToolsForceDisabledInRequest(req) {
		return promptcompat.ToolChoicePolicy{Mode: promptcompat.ToolChoiceNone}, nil
	}
	adapted := map[string]any{"tools": req["tools"]}
	if raw := req["tool_choice"]; raw != nil {
		choice, ok := raw.(map[string]any)
		if !ok {
			return promptcompat.ToolChoicePolicy{}, fmt.Errorf("tool_choice must be an object")
		}
		switch choice["type"] {
		case "auto", "none":
			adapted["tool_choice"] = choice["type"]
		case "any":
			adapted["tool_choice"] = "required"
		case "tool":
			adapted["tool_choice"] = map[string]any{"type": "function", "name": choice["name"]}
		default:
			return promptcompat.ToolChoicePolicy{}, fmt.Errorf("unsupported tool_choice.type: %v", choice["type"])
		}
		if v, exists := choice["disable_parallel_tool_use"]; exists {
			disabled, ok := v.(bool)
			if !ok {
				return promptcompat.ToolChoicePolicy{}, fmt.Errorf("disable_parallel_tool_use must be a boolean")
			}
			adapted["parallel_tool_calls"] = !disabled
		}
	}
	return promptcompat.ParseOpenAIToolPolicy(adapted)
}
