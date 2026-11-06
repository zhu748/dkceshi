package promptcompat

// These fields identify a caller-owned message boundary, even for an empty
// assistant body. Native Responses call IDs remain on their individual calls.
func copyPromptMessageMetadata(out, source map[string]any) {
	for _, key := range []string{"name", "id", "prefix", "tool_call_id", "is_error"} {
		if value, present := source[key]; present {
			out[key] = value
		}
	}
}

func hasPromptMessageMetadata(message map[string]any) bool {
	for _, key := range []string{"name", "id", "prefix", "refusal", "audio", "tool_call_id", "is_error"} {
		if message[key] != nil {
			return true
		}
	}
	return false
}

func hasExplicitPromptMessage(message map[string]any) bool {
	_, content := message["content"]
	return content || hasPromptMessageMetadata(message)
}
