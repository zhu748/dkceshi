package files

import (
	"fmt"
	"strings"
)

// The shared uploader accepts bytes and uploaded IDs. Reject unsupported image
// locators before prompt-only image cleanup can erase them without an upload.
func unsupportedImageSource(block map[string]any) error {
	kind, _ := block["type"].(string)
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "image", "image_url", "input_image":
	case "file", "input_file", "file_reference", "document":
		return fmt.Errorf("file input requires bytes, a data URL, or an uploaded file_id")
	default:
		return nil
	}
	values := []any{block["image_url"], block["url"]}
	if source, ok := block["source"].(map[string]any); ok && source["type"] == "url" {
		values = append(values, source["url"])
	}
	for _, value := range values {
		if object, ok := value.(map[string]any); ok {
			value = object["url"]
		}
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" && !isDataURL(text) {
			return fmt.Errorf("remote image URLs are not supported; provide an image data URL or an uploaded file_id")
		}
	}
	return fmt.Errorf("image input requires a data URL, a base64 source, or an uploaded file_id")
}
