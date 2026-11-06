package promptcompat

import "ds2api/internal/prompt"

func responsesReasoningMetadata(item map[string]any) string {
	metadata := make(map[string]any)
	for key, value := range item {
		switch key {
		case "type", "role", "id", "text", "content", "summary", "encrypted_content", "signature", "thoughtSignature", "thought_signature":
			continue
		}
		metadata[key] = value
	}
	return prompt.TextBlockMetadata(metadata)
}
