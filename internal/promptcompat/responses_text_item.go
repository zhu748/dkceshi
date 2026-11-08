package promptcompat

// A standalone text item is still a content block. Flattening it to text alone
// drops annotations, citations and future contextual fields before serialization.
func normalizeResponsesTextItem(item map[string]any, kind, role string) map[string]any {
	if kind != "text" && kind != "input_text" && kind != "output_text" {
		return nil
	}
	if role == "" {
		role = "user"
	}
	block := shallowCopyMap(item)
	// Message identity belongs to the envelope, not a second metadata block
	// after its text (which would also displace an assistant's prefill).
	for _, key := range []string{"role", "name", "id", "prefix", "tool_call_id", "is_error", "attachments", "files"} {
		delete(block, key)
	}
	out := map[string]any{"role": normalizeOpenAIRoleForPrompt(role), "content": []any{block}}
	copyPromptMessageMetadata(out, item)
	return out
}
