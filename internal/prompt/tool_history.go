package prompt

import (
	"encoding/json"
	"strings"
)

const opaqueToolHistoryLabel = "[Historical tool call with non-object arguments] "

// Presence is distinct from null. Aliases are checked by the input adapter;
// do not replace an explicitly supplied null with a different argument field.
func ToolCallArguments(call map[string]any) (any, bool) {
	fn, _ := call["function"].(map[string]any)
	for _, object := range []map[string]any{call, fn} {
		for _, key := range []string{"arguments", "input"} {
			if value, present := object[key]; present {
				return value, true
			}
		}
	}
	return nil, false
}

func opaqueToolCallHistory(call map[string]any, name string, arguments any) string {
	record := map[string]any{"name": name, "arguments": arguments}
	if id := asString(call["id"]); id != "" {
		record["call_id"] = id
	}
	data, err := json.Marshal(record)
	if err != nil {
		return ""
	}
	out := opaqueToolHistoryLabel + string(data)
	if text, ok := arguments.(string); ok {
		out += "\nArguments text (verbatim): " + renderPromptXMLText(text)
	}
	return out
}

// Keep decoded non-object historical arguments as labeled data, outside
// executable markup. Valid neighboring calls retain their order and IDs.
func formatToolCallHistory(calls []any) string {
	var parts, run []string
	flush := func() {
		if len(run) > 0 {
			parts = append(parts, promptEPSEToolCallsOpen+"\n"+strings.Join(run, "\n")+"\n"+promptEPSEToolCallsClose)
			run = nil
		}
	}
	for _, raw := range calls {
		call, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		block := formatToolCallForPrompt(call)
		if strings.HasPrefix(block, opaqueToolHistoryLabel) {
			flush()
			parts = append(parts, block)
		} else if block != "" {
			run = append(run, block)
		}
	}
	flush()
	if context := toolHistoryContext(calls); context != "" {
		parts = append([]string{context}, parts...)
	}
	return strings.Join(parts, "\n")
}
