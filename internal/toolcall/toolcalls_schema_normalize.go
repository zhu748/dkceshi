package toolcall

import (
	"encoding/json"
	"strings"
)

func NormalizeParsedToolCallsForSchemas(calls []ParsedToolCall, toolsRaw any) []ParsedToolCall {
	if len(calls) == 0 {
		return calls
	}
	schemas := buildToolSchemaIndex(toolsRaw)
	if len(schemas) == 0 && toolsRaw == nil {
		return calls
	}

	var changedAny bool
	out := make([]ParsedToolCall, len(calls))
	for i, call := range calls {
		out[i] = call
		schema, ok := schemas[strings.TrimSpace(call.Name)]
		if (!ok && !IsCustomTool(toolsRaw, call.Name)) || call.Input == nil {
			continue
		}
		input := restoreLiteralInput(call, schema, IsCustomTool(toolsRaw, call.Name))
		normalized, changed := normalizeToolValueWithSchema(input, schema)
		changed = changed || len(call.StringLiterals) > 0
		if !changed {
			continue
		}
		changedAny = true
		if input, ok := normalized.(map[string]any); ok {
			out[i].Input = input
		}
	}
	if !changedAny {
		return calls
	}
	return out
}

func buildToolSchemaIndex(toolsRaw any) map[string]any {
	tools, ok := toolsRaw.([]any)
	if !ok || len(tools) == 0 {
		return nil
	}
	out := make(map[string]any, len(tools))
	for _, item := range tools {
		tool, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _, schema := ExtractToolMeta(tool)
		if name == "" || schema == nil {
			continue
		}
		out[name] = schema
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func ExtractToolMeta(tool map[string]any) (string, string, any) {
	name := strings.TrimSpace(asStringValue(tool["name"]))
	desc := strings.TrimSpace(asStringValue(tool["description"]))
	schema := firstNonNil(
		tool["parameters"],
		tool["input_schema"],
		tool["inputSchema"],
		tool["schema"],
	)
	if fn, ok := tool["function"].(map[string]any); ok {
		if name == "" {
			name = strings.TrimSpace(asStringValue(fn["name"]))
		}
		if desc == "" {
			desc = strings.TrimSpace(asStringValue(fn["description"]))
		}
		schema = firstNonNil(
			schema,
			fn["parameters"],
			fn["input_schema"],
			fn["inputSchema"],
			fn["schema"],
		)
	}
	return name, desc, schema
}

func normalizeToolValueWithSchema(value any, schema any) (any, bool) {
	compiled, err := compileToolSchema(schema)
	if err != nil {
		return value, false
	}
	return normalizeCompiledToolValue(value, compiled, 0)
}

func shouldCoerceSchemaToString(schema map[string]any) bool {
	if schema == nil {
		return false
	}
	if isStringConst(schema["const"]) {
		return true
	}
	if isStringEnum(schema["enum"]) {
		return true
	}
	switch v := schema["type"].(type) {
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "string")
	case []any:
		return isOnlyStringLikeTypes(v)
	case []string:
		items := make([]any, 0, len(v))
		for _, item := range v {
			items = append(items, item)
		}
		return isOnlyStringLikeTypes(items)
	default:
		return false
	}
}

func isOnlyStringLikeTypes(values []any) bool {
	if len(values) == 0 {
		return false
	}
	hasString := false
	for _, item := range values {
		typ, ok := item.(string)
		if !ok {
			return false
		}
		switch strings.ToLower(strings.TrimSpace(typ)) {
		case "string":
			hasString = true
		case "null":
			continue
		default:
			return false
		}
	}
	return hasString
}

func isStringConst(v any) bool {
	_, ok := v.(string)
	return ok
}

func isStringEnum(v any) bool {
	values, ok := v.([]any)
	if !ok || len(values) == 0 {
		return false
	}
	for _, item := range values {
		if _, ok := item.(string); !ok {
			return false
		}
	}
	return true
}

func stringifySchemaValue(value any) (any, bool) {
	if value == nil {
		return value, false
	}
	if s, ok := value.(string); ok {
		return s, false
	}
	b, err := json.Marshal(value)
	if err != nil {
		return value, false
	}
	return string(b), true
}

func asStringValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}
