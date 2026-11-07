package prompt

// Preserve contextual fields while removing only the attachment's transport
// payload. Nested business metadata still uses the shared binary scrubber.
func AttachmentContextMetadata(block map[string]any) map[string]any {
	metadata := attachmentContextMetadata(block, conflictingAttachmentLabels(block))
	return sanitizeContentValue(metadata).(map[string]any)
}

func attachmentContextMetadata(block map[string]any, keepLabels map[string]bool) map[string]any {
	out := make(map[string]any)
	for key, value := range block {
		switch key {
		case "type", "id", "file_id", "data", "bytes", "base64", "file_data", "url", "encoding", "cache_control":
			continue
		case "filename", "file_name", "name", "mime_type", "mimeType", "content_type", "contentType", "media_type", "mediaType", "display_name", "displayName":
			if text, ok := value.(string); ok && keepLabels[key] {
				out[key] = text
			}
		case "source", "file", "inlineData", "inline_data", "image_url":
			if nested, ok := value.(map[string]any); ok {
				if metadata := attachmentContextMetadata(nested, keepLabels); len(metadata) > 0 {
					out[key] = metadata
				}
			}
		case "signature", "thoughtSignature", "thought_signature", "encrypted_content":
			out[key] = "[omitted_binary_payload]"
		default:
			out[key] = value
		}
	}
	return out
}
