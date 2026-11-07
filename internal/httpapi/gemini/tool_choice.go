package gemini

import (
	"ds2api/internal/promptcompat"
	"fmt"
	"reflect"
	"strings"
)

func geminiToolPolicy(req map[string]any, tools []any) (promptcompat.ToolChoicePolicy, error) {
	if promptcompat.ToolsForceDisabledInRequest(req) {
		return promptcompat.ToolChoicePolicy{Mode: promptcompat.ToolChoiceNone}, nil
	}
	choices, err := geminiToolChoices(req)
	if err != nil {
		return promptcompat.ToolChoicePolicy{}, err
	}
	var result *promptcompat.ToolChoicePolicy
	for _, choice := range choices {
		policy, err := parseGeminiToolChoice(choice, tools)
		if err != nil {
			return policy, err
		}
		if result != nil && !reflect.DeepEqual(*result, policy) {
			return policy, fmt.Errorf("conflicting Gemini tool configuration aliases")
		}
		result = &policy
	}
	return *result, nil
}

func geminiToolChoices(req map[string]any) ([]map[string]any, error) {
	var choices []map[string]any
	for _, key := range []string{"toolConfig", "tool_config"} {
		if req[key] == nil {
			continue
		}
		cfg, ok := req[key].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s must be an object", key)
		}
		before := len(choices)
		for _, name := range []string{"functionCallingConfig", "function_calling_config"} {
			if cfg[name] == nil {
				continue
			}
			choice, ok := cfg[name].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%s.%s must be an object", key, name)
			}
			choices = append(choices, choice)
		}
		if len(choices) == before {
			choices = append(choices, nil)
		}
	}
	if len(choices) == 0 {
		choices = append(choices, nil)
	}
	return choices, nil
}

func parseGeminiToolChoice(choice map[string]any, tools []any) (promptcompat.ToolChoicePolicy, error) {
	mode := ""
	if value, present := choice["mode"]; present {
		text, ok := value.(string)
		if !ok {
			return promptcompat.ToolChoicePolicy{}, fmt.Errorf("functionCallingConfig.mode must be a string")
		}
		mode = strings.ToUpper(strings.TrimSpace(text))
	}
	policy := map[string]any{"type": "auto"}
	switch mode {
	// MODE_UNSPECIFIED is part of the Gemini enum and behaves as AUTO.
	case "", "AUTO", "MODE_UNSPECIFIED", "VALIDATED":
	case "NONE":
		policy["type"] = "none"
	case "ANY":
		policy["type"] = "required"
	default:
		return promptcompat.ToolChoicePolicy{}, fmt.Errorf("unsupported functionCallingConfig.mode: %s", mode)
	}
	var result *promptcompat.ToolChoicePolicy
	for _, key := range []string{"allowedFunctionNames", "allowed_function_names"} {
		if names, present := choice[key]; present {
			policy["allowed_tools"] = names
			parsed, err := promptcompat.ParseOpenAIToolPolicy(map[string]any{"tools": tools, "tool_choice": policy})
			if err != nil {
				return parsed, err
			}
			if result != nil && !reflect.DeepEqual(*result, parsed) {
				return parsed, fmt.Errorf("conflicting allowed function name aliases")
			}
			result = &parsed
		}
	}
	if result != nil {
		return *result, nil
	}
	return promptcompat.ParseOpenAIToolPolicy(map[string]any{"tools": tools, "tool_choice": policy})
}
