package prompt

import (
	"encoding/json"
	"strings"
)

// FileReferenceText preserves where an uploaded attachment occurred without
// leaking its bytes into text. IDs correspond to the request's ref_file_ids.
func FileReferenceText(block map[string]any) string {
	kind, _ := block["type"].(string)
	kind = strings.ToLower(strings.TrimSpace(kind))
	switch kind {
	case "image", "image_url", "input_image", "file", "input_file", "file_reference", "document":
	default:
		return ""
	}
	id, err := ResolveFileReferenceID(block)
	if err != nil || id == "" {
		return ""
	}
	sources := []map[string]any{block}
	for nested, ok := block["file"].(map[string]any); ok; nested, ok = nested["file"].(map[string]any) {
		sources = append(sources, nested)
	}
	ref := map[string]any{"type": kind, "file_id": id}
	for _, aliases := range attachmentLabelAliases() {
		for i := len(sources) - 1; i >= 0; i-- {
			object := sources[i]
			for _, key := range aliases {
				if value, ok := object[key].(string); ok && value != "" && ref[aliases[0]] == nil {
					ref[aliases[0]] = value
				}
			}
		}
	}
	if metadata := AttachmentContextMetadata(block); len(metadata) > 0 {
		ref["metadata"] = metadata
	}
	data, _ := json.Marshal(ref)
	return "[Attachment reference] " + string(data)
}
