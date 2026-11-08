package prompt

import (
	"encoding/json"
	"strings"
)

// ToolCallContextFields separates call/function extensions from executable
// names and arguments. The source maps and all argument values stay untouched.
func ToolCallContextFields(call map[string]any) map[string]any {
	function, _ := call["function"].(map[string]any)
	outer := toolContextExtras(call, "id", "name", "type", "arguments", "input", "function")
	inner := toolContextExtras(function, "name", "arguments", "input")
	if len(outer)+len(inner) == 0 {
		return nil
	}
	fields := make(map[string]any, 2)
	if len(outer) > 0 {
		fields["call"] = outer
	}
	if len(inner) > 0 {
		fields["function"] = inner
	}
	return fields
}

func toolContextExtras(object map[string]any, consumed ...string) map[string]any {
	var fields map[string]any
	for key, value := range object {
		skip := key == "cache_control"
		for _, field := range consumed {
			skip = skip || field == key
		}
		if skip {
			continue
		}
		if fields == nil {
			fields = make(map[string]any)
		}
		fields[key] = value
	}
	return fields
}

func toolHistoryContext(calls []any) string {
	var records []any
	for index, raw := range calls {
		call, _ := raw.(map[string]any)
		fields := ToolCallContextFields(call)
		if len(fields) == 0 {
			continue
		}
		name := strings.TrimSpace(asString(call["name"]))
		if function, ok := call["function"].(map[string]any); name == "" && ok {
			name = strings.TrimSpace(asString(function["name"]))
		}
		if name == "" {
			continue
		}
		record := map[string]any{"index": index, "name": name, "fields": fields}
		if id := asString(call["id"]); id != "" {
			record["call_id"] = id
		}
		records = append(records, record)
	}
	if len(records) == 0 {
		return ""
	}
	data, err := json.Marshal(SanitizeContextValue(records))
	if err != nil {
		return ""
	}
	return "[Historical tool call context (JSON data)] " + string(data)
}
