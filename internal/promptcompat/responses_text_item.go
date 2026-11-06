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
	out := map[string]any{"role": normalizeOpenAIRoleForPrompt(role), "content": []any{item}}
	copyPromptMessageMetadata(out, item)
	return out
}
