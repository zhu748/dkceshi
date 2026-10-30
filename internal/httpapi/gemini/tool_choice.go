package gemini

import (
	"ds2api/internal/promptcompat"
	"fmt"
	"strings"
)

func geminiToolPolicy(req map[string]any, tools []any) (promptcompat.ToolChoicePolicy, error) {
	if promptcompat.ToolsForceDisabledInRequest(req) {
		return promptcompat.ToolChoicePolicy{Mode: promptcompat.ToolChoiceNone}, nil
	}
	adapted := map[string]any{"tools": tools}
	cfg, _ := req["toolConfig"].(map[string]any)
	if cfg == nil {
		cfg, _ = req["tool_config"].(map[string]any)
	}
	choice, _ := cfg["functionCallingConfig"].(map[string]any)
	if choice == nil {
		choice, _ = cfg["function_calling_config"].(map[string]any)
	}
	mode := strings.ToUpper(asString(choice["mode"]))
	policy := map[string]any{"type": "auto"}
	switch mode {
	case "", "AUTO", "VALIDATED":
	case "NONE":
		policy["type"] = "none"
	case "ANY":
		policy["type"] = "required"
	default:
		return promptcompat.ToolChoicePolicy{}, fmt.Errorf("unsupported functionCallingConfig.mode: %s", mode)
	}
	names := choice["allowedFunctionNames"]
	if names == nil {
		names = choice["allowed_function_names"]
	}
	if names != nil {
		policy["allowed_tools"] = names
	}
	adapted["tool_choice"] = policy
	return promptcompat.ParseOpenAIToolPolicy(adapted)
}
