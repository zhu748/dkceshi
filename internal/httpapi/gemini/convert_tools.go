package gemini

import "strings"

//nolint:unused // kept for native Gemini adapter route compatibility.
func convertGeminiTools(raw any) []any {
	tools, _ := raw.([]any)
	if len(tools) == 0 {
		return tools
	}
	out := make([]any, 0, len(tools))
	for _, item := range tools {
		tool, ok := item.(map[string]any)
		if !ok {
			out = append(out, item)
			continue
		}

		if fnDecls, ok := tool["functionDeclarations"].([]any); ok && len(fnDecls) > 0 {
			for _, declRaw := range fnDecls {
				decl, ok := declRaw.(map[string]any)
				if !ok {
					out = append(out, declRaw)
					continue
				}
				name := strings.TrimSpace(asString(decl["name"]))
				if name == "" {
					out = append(out, decl)
					continue
				}
				function := map[string]any{
					"name": name,
				}
				if desc := strings.TrimSpace(asString(decl["description"])); desc != "" {
					function["description"] = desc
				}
				if params, exists := decl["parameters"]; exists {
					function["parameters"] = geminiParameterSchema(params)
				}
				if schema, exists := decl["parametersJsonSchema"]; exists {
					function["parameters"] = schema
				}
				out = append(out, map[string]any{
					"type":     "function",
					"function": function,
				})
			}
			continue
		}

		// OpenAI-style passthrough fallback.
		if _, ok := tool["function"].(map[string]any); ok {
			out = append(out, tool)
			continue
		}

		// Loose fallback for flattened function schema objects.
		name := strings.TrimSpace(asString(tool["name"]))
		if name == "" {
			out = append(out, tool)
			continue
		}
		fn := map[string]any{"name": name}
		if desc := strings.TrimSpace(asString(tool["description"])); desc != "" {
			fn["description"] = desc
		}
		if params, exists := tool["parameters"]; exists {
			fn["parameters"] = geminiParameterSchema(params)
		}
		out = append(out, map[string]any{
			"type":     "function",
			"function": fn,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func geminiParameterSchema(raw any) any {
	if schema, ok := raw.(map[string]any); ok {
		return normalizeGeminiSchema(schema)
	}
	return raw
}
