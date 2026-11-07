package prompt

func attachmentLabelAliases() [][]string {
	return [][]string{
		{"filename", "file_name", "name"},
		{"mime_type", "mimeType", "content_type", "contentType", "media_type", "mediaType"},
		{"display_name", "displayName"},
	}
}

func attachmentContainerKeys() []string {
	return []string{"source", "file", "inlineData", "inline_data", "image_url"}
}

// Different names can be useful caller context rather than equivalent
// transport spellings. Keep their original paths whenever one chosen label
// cannot represent every source value.
func conflictingAttachmentLabels(block map[string]any) map[string]bool {
	aliases := attachmentLabelAliases()
	first := make([]string, len(aliases))
	conflicts := make([]bool, len(aliases))
	var visit func(map[string]any)
	visit = func(object map[string]any) {
		for i, group := range aliases {
			for _, key := range group {
				if text, ok := object[key].(string); ok && text != "" {
					conflicts[i] = conflicts[i] || first[i] != "" && first[i] != text
					first[i] = text
				}
			}
		}
		for _, key := range attachmentContainerKeys() {
			if nested, ok := object[key].(map[string]any); ok {
				visit(nested)
			}
		}
	}
	visit(block)
	keep := make(map[string]bool)
	for i, conflict := range conflicts {
		if conflict {
			for _, key := range aliases[i] {
				keep[key] = true
			}
		}
	}
	return keep
}
