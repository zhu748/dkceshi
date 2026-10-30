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
	case "image", "image_url", "input_image", "file", "input_file":
	default:
		return ""
	}
	source := block
	if nested, ok := block["file"].(map[string]any); ok {
		source = nested
	}
	id, _ := source["file_id"].(string)
	if strings.TrimSpace(id) == "" {
		return ""
	}
	ref := map[string]any{"type": kind, "file_id": id}
	for _, key := range []string{"filename", "mime_type"} {
		if value, ok := source[key].(string); ok && value != "" {
			ref[key] = value
		}
	}
	data, _ := json.Marshal(ref)
	return "[Attachment reference] " + string(data)
}
