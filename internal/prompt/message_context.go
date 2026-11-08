package prompt

import (
	"encoding/json"
	"strings"
)

func MessageContextText(message map[string]any) string {
	fields := MessageContextFields(message)
	if len(fields) == 0 {
		return ""
	}
	encoded, err := json.Marshal(SanitizeContextValue(fields))
	if err != nil {
		return ""
	}
	return "Message context (JSON data): " + string(encoded) + "\n"
}

// MessageContextFields keeps extensions separate from the role and body. The
// renderer places their JSON before the role header so prefill stays last.
// Callers adapting a native envelope remove its consumed protocol fields first.
func MessageContextFields(message map[string]any) map[string]any {
	role, _ := message["role"].(string)
	assistant := strings.EqualFold(strings.TrimSpace(role), "assistant") || strings.EqualFold(strings.TrimSpace(role), "model")
	var fields map[string]any
	for key, value := range message {
		switch key {
		case "role", "content", "name", "id", "prefix", "tool_call_id", "is_error", "attachments", "files":
			continue
		case "cache_control", "signature", "thoughtSignature", "thought_signature", "encrypted_content":
			continue // Transport hints and opaque provider state have no text meaning.
		case "reasoning_content", "tool_calls", "function_call", "refusal", "audio":
			if assistant {
				continue
			}
		}
		if fields == nil {
			fields = make(map[string]any)
		}
		fields[key] = value
	}
	return fields
}

// ContextValueOmittedPaths describes only actual binary/opaque substitutions.
func ContextValueOmittedPaths(value any) []string {
	var paths []string
	collectOmittedPaths(value, sanitizeContentValue(value), "", &paths)
	return paths
}
