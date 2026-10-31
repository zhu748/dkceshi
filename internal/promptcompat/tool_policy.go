package promptcompat

import (
	"ds2api/internal/toolcall"
	"fmt"
	"strings"
)

func parseOpenAIToolPolicy(req map[string]any) (ToolChoicePolicy, error) {
	return ParseOpenAIToolPolicy(req)
}

// ParseOpenAIToolPolicy is shared by adapters after mapping their policy fields.
func ParseOpenAIToolPolicy(req map[string]any) (ToolChoicePolicy, error) {
	if err := toolcall.ValidateToolSchemas(req["tools"]); err != nil {
		return ToolChoicePolicy{}, err
	}
	policy, err := parseToolChoicePolicy(req["tool_choice"], req["tools"])
	if err != nil {
		return policy, err
	}
	if value, exists := req["parallel_tool_calls"]; exists && value != nil {
		parallel, ok := value.(bool)
		if !ok {
			return ToolChoicePolicy{}, fmt.Errorf("parallel_tool_calls must be a boolean")
		}
		policy.DisableParallel = !parallel
	}
	return policy, nil
}

func parseToolChoicePolicy(toolChoiceRaw any, toolsRaw any) (ToolChoicePolicy, error) {
	policy := DefaultToolChoicePolicy()
	declaredNames := extractDeclaredToolNames(toolsRaw)
	declaredSet := namesToSet(declaredNames)
	if len(declaredNames) > 0 {
		policy.Allowed = declaredSet
		policy.RestrictAllowed = true
	}

	if toolChoiceRaw == nil {
		return policy, nil
	}

	switch v := toolChoiceRaw.(type) {
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "", "auto":
			policy.Mode = ToolChoiceAuto
		case "none":
			policy.Mode = ToolChoiceNone
			policy.Allowed = nil
		case "required":
			policy.Mode = ToolChoiceRequired
		default:
			return ToolChoicePolicy{}, fmt.Errorf("unsupported tool_choice: %q", v)
		}
	case map[string]any:
		if strings.EqualFold(asString(v["type"]), "allowed_tools") {
			mode := asString(v["mode"])
			if mode != "auto" && mode != "required" {
				return ToolChoicePolicy{}, fmt.Errorf("tool_choice.allowed_tools mode must be auto or required")
			}
			if v["tools"] == nil {
				return ToolChoicePolicy{}, fmt.Errorf("tool_choice.allowed_tools requires tools")
			}
			v = map[string]any{"type": mode, "allowed_tools": v["tools"]}
		}
		allowedOverride, hasAllowedOverride, err := parseAllowedToolNames(v["allowed_tools"])
		if err != nil {
			return ToolChoicePolicy{}, err
		}
		if hasAllowedOverride {
			policy.RestrictAllowed = true
			filtered := make([]string, 0, len(allowedOverride))
			for _, name := range allowedOverride {
				if _, ok := declaredSet[name]; !ok {
					return ToolChoicePolicy{}, fmt.Errorf("tool_choice.allowed_tools contains undeclared tool %q", name)
				}
				filtered = append(filtered, name)
			}
			policy.Allowed = namesToSet(filtered)
		}

		typ := strings.ToLower(strings.TrimSpace(asString(v["type"])))
		switch typ {
		case "", "auto":
			if hasFunctionSelector(v) {
				name, err := parseForcedToolName(v)
				if err != nil {
					return ToolChoicePolicy{}, err
				}
				policy.Mode = ToolChoiceForced
				policy.ForcedName = name
				policy.Allowed = namesToSet([]string{name})
			} else {
				policy.Mode = ToolChoiceAuto
			}
		case "none":
			policy.Mode = ToolChoiceNone
			policy.Allowed = nil
		case "required":
			policy.Mode = ToolChoiceRequired
		case "function":
			name, err := parseForcedToolName(v)
			if err != nil {
				return ToolChoicePolicy{}, err
			}
			policy.Mode = ToolChoiceForced
			policy.ForcedName = name
			policy.Allowed = namesToSet([]string{name})
		default:
			return ToolChoicePolicy{}, fmt.Errorf("unsupported tool_choice.type: %q", typ)
		}
		if hasAllowedOverride && policy.Mode == ToolChoiceForced {
			if _, ok := namesToSet(allowedOverride)[policy.ForcedName]; !ok {
				return ToolChoicePolicy{}, fmt.Errorf("tool_choice forced function %q is excluded by allowed_tools", policy.ForcedName)
			}
		}
	default:
		return ToolChoicePolicy{}, fmt.Errorf("tool_choice must be a string or object")
	}

	if policy.Mode == ToolChoiceRequired || policy.Mode == ToolChoiceForced {
		if len(declaredNames) == 0 {
			return ToolChoicePolicy{}, fmt.Errorf("tool_choice=%s requires non-empty tools", policy.Mode)
		}
	}
	if policy.Mode == ToolChoiceForced {
		if _, ok := declaredSet[policy.ForcedName]; !ok {
			return ToolChoicePolicy{}, fmt.Errorf("tool_choice forced function %q is not declared in tools", policy.ForcedName)
		}
	}
	if len(policy.Allowed) == 0 && (policy.Mode == ToolChoiceRequired || policy.Mode == ToolChoiceForced) {
		return ToolChoicePolicy{}, fmt.Errorf("tool_choice policy resolved to empty allowed tool set")
	}
	return policy, nil
}

func parseForcedToolName(v map[string]any) (string, error) {
	if name := strings.TrimSpace(asString(v["name"])); name != "" {
		return name, nil
	}
	if fn, ok := v["function"].(map[string]any); ok {
		if name := strings.TrimSpace(asString(fn["name"])); name != "" {
			return name, nil
		}
	}
	return "", fmt.Errorf("tool_choice function requires name")
}

func parseAllowedToolNames(raw any) ([]string, bool, error) {
	if raw == nil {
		return nil, false, nil
	}
	collectName := func(v any) string {
		if name := strings.TrimSpace(asString(v)); name != "" {
			return name
		}
		if m, ok := v.(map[string]any); ok {
			if name := strings.TrimSpace(asString(m["name"])); name != "" {
				return name
			}
			if fn, ok := m["function"].(map[string]any); ok {
				if name := strings.TrimSpace(asString(fn["name"])); name != "" {
					return name
				}
			}
		}
		return ""
	}

	names := []string{}
	switch x := raw.(type) {
	case []any:
		for _, item := range x {
			name := collectName(item)
			if name == "" {
				return nil, true, fmt.Errorf("tool_choice.allowed_tools contains invalid item")
			}
			names = append(names, name)
		}
	case []string:
		for _, item := range x {
			name := strings.TrimSpace(item)
			if name == "" {
				return nil, true, fmt.Errorf("tool_choice.allowed_tools contains empty name")
			}
			names = append(names, name)
		}
	default:
		return nil, true, fmt.Errorf("tool_choice.allowed_tools must be an array")
	}

	if len(names) == 0 {
		return nil, true, fmt.Errorf("tool_choice.allowed_tools must not be empty")
	}
	return names, true, nil
}

func hasFunctionSelector(v map[string]any) bool {
	if strings.TrimSpace(asString(v["name"])) != "" {
		return true
	}
	if fn, ok := v["function"].(map[string]any); ok {
		return strings.TrimSpace(asString(fn["name"])) != ""
	}
	return false
}

func extractDeclaredToolNames(toolsRaw any) []string {
	tools, ok := toolsRaw.([]any)
	if !ok || len(tools) == 0 {
		return nil
	}
	out := make([]string, 0, len(tools))
	seen := map[string]struct{}{}
	for _, t := range tools {
		tool, ok := t.(map[string]any)
		if !ok {
			continue
		}
		fn, _ := tool["function"].(map[string]any)
		if len(fn) == 0 {
			fn = tool
		}
		name := strings.TrimSpace(asString(fn["name"]))
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

func namesToSet(names []string) map[string]struct{} {
	if len(names) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		out[trimmed] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
