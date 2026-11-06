package gemini

import "ds2api/internal/promptcompat"

// Keep the JSON response and its ordered media parts in one standard tool
// message. The shared upload path replaces media bytes with file references.
func geminiFunctionResponseContent(response map[string]any) any {
	content, hasPayload := response["response"]
	if !hasPayload {
		content, hasPayload = response["output"]
	}
	if !hasPayload {
		content = ""
	}
	parts, _ := response["parts"].([]any)
	if len(parts) == 0 {
		return content
	}
	if content == nil {
		content = "null"
	}
	out := make([]any, 0, len(parts)+1)
	out = append(out, map[string]any{"type": "text", "text": promptcompat.NormalizeOpenAIContentForPrompt(content)})
	for _, part := range parts {
		block, ok := part.(map[string]any)
		if !ok {
			out = append(out, part)
			continue
		}
		if attachment, ok := promptcompat.NormalizeAttachmentContent(block); ok {
			out = append(out, attachment)
		} else {
			out = append(out, map[string]any{"type": "text", "text": formatGeminiUnknownPartForPrompt(block)})
		}
	}
	return out
}
