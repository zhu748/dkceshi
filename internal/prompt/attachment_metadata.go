package prompt

// Preserve contextual fields while removing only the attachment's transport
// payload. Nested business metadata still uses the shared binary scrubber.
func AttachmentContextMetadata(block map[string]any) map[string]any {
	out := make(map[string]any)
	for key, value := range block {
		switch key {
		case "type", "id", "file_id", "filename", "file_name", "name", "mime_type", "mimeType", "content_type", "contentType", "media_type", "mediaType", "display_name", "displayName", "data", "bytes", "base64", "file_data", "url", "encoding", "cache_control":
			continue
		case "source", "file", "inlineData", "inline_data", "image_url":
			if nested, ok := value.(map[string]any); ok {
				if metadata := AttachmentContextMetadata(nested); len(metadata) > 0 {
					out[key] = metadata
				}
			}
		default:
			out[key] = sanitizeContentValue(value)
		}
	}
	return sanitizeContentValue(out).(map[string]any)
}
