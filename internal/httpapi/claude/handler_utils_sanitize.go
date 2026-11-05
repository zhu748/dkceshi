package claude

import "ds2api/internal/prompt"

const omittedBinaryMarker = "[omitted_binary_payload]"

// Unknown structured context follows the same text/binary rules as Chat and
// Responses. Input limits are enforced by the request reader, not truncation.
func formatClaudeUnknownBlockForPrompt(block map[string]any) string {
	if block == nil {
		return ""
	}
	return formatClaudeBlockRaw(sanitizeClaudeBlockForPrompt(block))
}

func sanitizeClaudeBlockForPrompt(block map[string]any) map[string]any {
	return prompt.SanitizeContextValue(block).(map[string]any)
}

func sanitizeClaudeStringForPrompt(key, value string) string {
	return prompt.SanitizeContextValue(map[string]any{key: value}).(map[string]any)[key].(string)
}
