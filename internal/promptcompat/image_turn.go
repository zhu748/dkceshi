package promptcompat

import "strings"

// A referenced image still represents a user turn even with no accompanying
// text. Only uploaded/reference blocks qualify; unsupported URLs do not.
func hasImageFileReference(raw any) bool {
	switch value := raw.(type) {
	case map[string]any:
		return isImageContentBlock(value) && strings.TrimSpace(asString(value["file_id"])) != ""
	case []any:
		for _, item := range value {
			if hasImageFileReference(item) {
				return true
			}
		}
	case []map[string]any:
		for _, item := range value {
			if hasImageFileReference(item) {
				return true
			}
		}
	}
	return false
}
