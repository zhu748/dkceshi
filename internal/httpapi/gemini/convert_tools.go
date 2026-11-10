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
				function := geminiFunctionDefinition(decl)
				entry := map[string]any{"type": "function", "function": function}
				if context := geminiCatalogContext(tool); len(context) > 0 {
					entry["native_catalog_context"] = context
				}
				out = append(out, entry)
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
		fn := geminiFunctionDefinition(tool)
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

func geminiCatalogContext(tool map[string]any) map[string]any {
	context := make(map[string]any)
	for key, value := range tool {
		if key != "functionDeclarations" && key != "cache_control" {
			context[key] = value
		}
	}
	return context
}

// Keep every schema spelling until shared alias validation has compared them.
// Only Gemini's native parameters field uses the uppercase Schema dialect.
func geminiFunctionDefinition(source map[string]any) map[string]any {
	function := make(map[string]any, len(source))
	for key, value := range source {
		function[key] = value
	}
	if params, exists := source["parameters"]; exists {
		function["parameters"] = geminiParameterSchema(params)
	}
	return function
}

func geminiParameterSchema(raw any) any {
	if schema, ok := raw.(map[string]any); ok {
		return normalizeGeminiSchema(schema)
	}
	return raw
}
