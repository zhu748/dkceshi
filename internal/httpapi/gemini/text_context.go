package gemini

// Preserve text-part metadata through the shared content serializer. Thought
// flags and opaque signatures are transport state, not textual context.
func geminiTextBlock(part map[string]any, text string) any {
	block := map[string]any{"type": "text", "text": text}
	for key, value := range part {
		switch key {
		case "text", "type", "thought", "thoughtSignature", "thought_signature":
			continue
		default:
			block[key] = value
		}
	}
	if len(block) == 2 {
		return text
	}
	return block
}
