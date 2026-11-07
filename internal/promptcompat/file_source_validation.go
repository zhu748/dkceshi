package promptcompat

import (
	"fmt"
	"strings"
)

func validateFileSource(block map[string]any, kind string) error {
	file, _ := block["file"].(map[string]any)
	if (kind == "file" || kind == "input_file" || kind == "file_reference") && strings.TrimSpace(asString(block["id"])) != "" {
		return nil
	}
	if strings.TrimSpace(asString(block["file_id"])) != "" || strings.TrimSpace(asString(file["file_id"])) != "" || strings.TrimSpace(asString(file["id"])) != "" {
		return nil // Uploaded IDs do not require replaying stale locator hints.
	}
	switch kind {
	case "file", "input_file", "file_reference":
		for _, key := range []string{"file_url", "url"} {
			url := strings.TrimSpace(asString(block[key]))
			if url != "" && !strings.HasPrefix(strings.ToLower(url), "data:") {
				return fmt.Errorf("remote file URLs are not supported; provide a file data URL or an uploaded file_id")
			}
		}
	case "document":
		source, _ := block["source"].(map[string]any)
		if source["type"] == "url" {
			return fmt.Errorf("document URL sources are not supported; provide a base64 source or an uploaded file_id")
		}
	}
	return nil
}
