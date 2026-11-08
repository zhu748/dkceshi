package promptcompat

import "strings"

// Keep every native source until shared preprocessing validates and uploads it.
// Adapters must not select a byte alias or discard an authoritative uploaded ID.
func NormalizeAttachmentContent(block map[string]any) (map[string]any, bool) {
	kind := strings.ToLower(strings.TrimSpace(asString(block["type"])))
	if kind == "image_url" || kind == "input_image" || kind == "input_file" || kind == "file" || kind == "file_reference" {
		return block, true
	}
	source, _ := block["source"].(map[string]any)
	if kind == "document" && (source["type"] == "text" || source["type"] == "content") {
		return nil, false // Text documents belong to the textual context serializer.
	}
	if kind != "image" && kind != "document" {
		_, camel := block["inlineData"]
		_, snake := block["inline_data"]
		if !camel && !snake {
			return nil, false
		}
		source, _ = block["inlineData"].(map[string]any)
		if !camel {
			source, _ = block["inline_data"].(map[string]any)
		}
	}
	out := shallowCopyMap(block)
	if kind == "image" || kind == "document" {
		out["type"] = kind
	} else {
		out["type"] = "input_file"
		for _, key := range []string{"mime_type", "mimeType", "content_type", "contentType", "media_type", "mediaType"} {
			if mime := strings.ToLower(strings.TrimSpace(asString(source[key]))); mime != "" {
				if strings.HasPrefix(mime, "image/") {
					out["type"] = "input_image"
				}
				break
			}
		}
	}
	return attachmentWithContext(out, block), true
}

func attachmentWithContext(converted, original map[string]any) map[string]any {
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
