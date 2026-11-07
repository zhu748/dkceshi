package promptcompat

import (
	"strings"

	"ds2api/internal/prompt"
)

// NormalizeAttachmentContent retains uploadable bytes until shared file
// preprocessing runs. Prompt rendering omits binary payloads before that step.
func NormalizeAttachmentContent(block map[string]any) (map[string]any, bool) {
	kind := asString(block["type"])
	if kind == "image_url" || kind == "input_image" || kind == "input_file" || kind == "file" {
		return block, true
	}
	source, _ := block["source"].(map[string]any)
	image := kind == "image"
	if kind != "image" && kind != "document" {
		source, _ = block["inlineData"].(map[string]any)
		if source == nil {
			source, _ = block["inline_data"].(map[string]any)
		}
	}
	if source == nil {
		return nil, false
	}
	if source["type"] == "url" && image {
		return attachmentWithContext(map[string]any{"type": "image_url", "image_url": map[string]any{"url": source["url"]}}, block), true
	}
	mime := asString(source["media_type"])
	if mime == "" {
		mime = asString(source["mimeType"])
	}
	if mime == "" {
		mime = asString(source["mime_type"])
	}
	if mime == "" {
		mime = "application/octet-stream"
	}
	data, ok := source["data"].(string)
	if !ok || (source["type"] != nil && source["type"] != "base64") {
		return nil, false
	}
	url := "data:" + mime + ";base64," + data
	if image || strings.HasPrefix(mime, "image/") {
		return attachmentWithContext(map[string]any{"type": "image_url", "image_url": map[string]any{"url": url}}, block), true
	}
	return attachmentWithContext(map[string]any{"type": "input_file", "file_data": url, "filename": block["filename"]}, block), true
}

func attachmentWithContext(converted, original map[string]any) map[string]any {
	if metadata := prompt.AttachmentContextMetadata(original); len(metadata) > 0 {
		converted["context_metadata"] = metadata
	}
	for _, key := range []string{"", "source", "inlineData", "inline_data"} {
		source := original
		if key != "" {
			source, _ = original[key].(map[string]any)
		}
		for _, aliases := range [][]string{{"filename", "file_name", "name"}, {"display_name", "displayName"}} {
			for _, name := range aliases {
				if text, ok := source[name].(string); ok && text != "" && asString(converted[aliases[0]]) == "" {
					converted[aliases[0]] = text
				}
			}
		}
	}
	return converted
}

// JoinContentText keeps exact text concatenation, and structured attachment
// positions when a message mixes text with files.
func JoinContentText(parts []any) any {
	var b strings.Builder
	textOnly := true
	for _, part := range parts {
		if text, ok := part.(string); ok {
			b.WriteString(text)
		} else {
			textOnly = false
		}
	}
	if textOnly {
		return b.String()
	}
	out := make([]any, 0, len(parts))
	for _, part := range parts {
		if text, ok := part.(string); ok {
			part = map[string]any{"type": "text", "text": text}
		}
		out = append(out, part)
	}
	return out
}
