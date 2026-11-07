package prompt

// Native inline media has both a MIME type and a payload. A business key alone
// cannot establish this identity, and no base64/length heuristic is used.
func explicitInlineMedia(value any) bool {
	block, ok := value.(map[string]any)
	if !ok {
		return false
	}
	for _, field := range []string{"mime_type", "mimeType", "media_type", "mediaType"} {
		if mime, ok := block[field].(string); ok && mime != "" {
			for _, payload := range []string{"data", "bytes", "base64", "fileUri", "file_uri"} {
				if _, present := block[payload]; present {
					return true
				}
			}
		}
	}
	return false
}
