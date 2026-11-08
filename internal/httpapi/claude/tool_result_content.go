package claude

import (
	"ds2api/internal/prompt"
	"ds2api/internal/promptcompat"
)

func claudeToolResultAttachments(content any) (any, bool) {
	blocks, ok := content.([]any)
	if !ok || !promptcompat.ContentHasAttachments(blocks) {
		return nil, false
	}
	out := make([]any, len(blocks))
	for i, raw := range blocks {
		out[i] = raw
		if block, ok := raw.(map[string]any); ok {
			if attachment, ok := promptcompat.NormalizeAttachmentContent(block); ok {
				out[i] = attachment
			}
		}
	}
	return out, true
}

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
		if block, ok := prompt.PreserveTextContextBlock(value); ok {
			return block
		}
		return sanitizeClaudeBlockForPrompt(value)
	default:
		return content
	}
}
