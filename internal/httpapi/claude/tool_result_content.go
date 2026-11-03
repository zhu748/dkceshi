package claude

// Text blocks are documented tool output, including file bodies and command
// output. Never apply unknown-block truncation or base64 guessing to their text.
// Unrecognized blocks retain the existing binary-payload protection.
func preserveClaudeToolResultContent(content any) any {
	switch value := content.(type) {
	case []any:
		out := make([]any, len(value))
		for i, item := range value {
			out[i] = preserveClaudeToolResultContent(item)
		}
		return out
	case map[string]any:
		out := sanitizeClaudeBlockForPrompt(value)
		if value["type"] == "text" {
			if text, ok := value["text"].(string); ok {
				out["text"] = text
			}
		}
		return out
	default:
		return content
	}
}
