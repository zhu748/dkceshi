package prompt

import "encoding/json"

// TextBlockMetadata preserves citations and other contextual fields alongside
// the exact text. Cache hints describe transport rather than message content.
func TextBlockMetadata(block map[string]any) string {
	return blockMetadata(block, "type", "text", "content", "cache_control")
}

func RefusalBlockMetadata(block map[string]any) string {
	return blockMetadata(block, "type", "refusal", "cache_control")
}

// Thinking signatures are opaque provider state; contextual annotations remain
// next to their exact reasoning text in both live prompts and history files.
func ReasoningBlockMetadata(block map[string]any) string {
	return blockMetadata(block, "type", "text", "thinking", "content", "cache_control", "signature", "thoughtSignature", "thought_signature")
}

func blockMetadata(block map[string]any, excluded ...string) string {
	metadata := make(map[string]any)
	for key, value := range block {
		skip := false
		for _, field := range excluded {
			skip = skip || key == field
		}
		if skip {
			continue
		}
		metadata[key] = value
	}
	if len(metadata) == 0 {
		return ""
	}
	data, err := json.Marshal(sanitizeContentValue(metadata))
	if err != nil {
		return ""
	}
	return "\n[Content metadata] " + string(data) + "\n"
}
