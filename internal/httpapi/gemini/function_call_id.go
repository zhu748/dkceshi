package gemini

import "strings"

// Both sides of a tool round must use exactly the same ID alias precedence.
func geminiFunctionCallID(fn map[string]any) string {
	for _, key := range []string{"id", "call_id", "callId", "tool_call_id"} {
		if id := strings.TrimSpace(asString(fn[key])); id != "" {
			return id
		}
	}
	return ""
}
