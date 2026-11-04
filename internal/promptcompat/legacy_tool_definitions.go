package promptcompat

import (
	"fmt"
	"reflect"
)

// NormalizeLegacyToolDefinitions translates request controls before policy and
// prompt construction. History conversion alone cannot declare callable tools.
// The caller's request and nested definitions remain untouched.
func NormalizeLegacyToolDefinitions(req map[string]any) (map[string]any, error) {
	if req["functions"] == nil && req["function_call"] == nil {
		return req, nil
	}
	out := make(map[string]any, len(req)+2)
	for key, value := range req {
		out[key] = value
	}
	if raw := req["functions"]; raw != nil {
		functions, ok := raw.([]any)
		if !ok {
			return nil, fmt.Errorf("functions must be an array")
		}
		tools := make([]any, 0, len(functions))
		for i, raw := range functions {
			fn, ok := raw.(map[string]any)
			if !ok || len(fn) == 0 {
				return nil, fmt.Errorf("functions[%d] must be a function object", i)
			}
			tools = append(tools, map[string]any{"type": "function", "function": fn})
		}
		if modern, present := req["tools"]; present && modern != nil && !reflect.DeepEqual(modern, tools) {
			return nil, fmt.Errorf("functions conflicts with tools")
		}
		out["tools"] = tools
	}
	if raw := req["function_call"]; raw != nil {
		var choice any
		switch value := raw.(type) {
		case string:
			if value != "auto" && value != "none" {
				return nil, fmt.Errorf("function_call must be auto, none or a named function object")
			}
			choice = value
		case map[string]any:
			if asString(value["name"]) == "" {
				return nil, fmt.Errorf("function_call requires a function name")
			}
			choice = map[string]any{"type": "function", "function": map[string]any{"name": value["name"]}}
		default:
			return nil, fmt.Errorf("function_call must be a string or object")
		}
		if modern := req["tool_choice"]; modern != nil && !reflect.DeepEqual(modern, choice) {
			return nil, fmt.Errorf("function_call conflicts with tool_choice")
		}
		out["tool_choice"] = choice
	}
	delete(out, "functions")
	delete(out, "function_call")
	return out, nil
}
