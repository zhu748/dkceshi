package prompt

import "encoding/json"

// TextBlockMetadata preserves citations and other contextual fields alongside
// the exact text. Cache hints describe transport rather than message content.
func TextBlockMetadata(block map[string]any) string {
	metadata := make(map[string]any)
	for key, value := range block {
		switch key {
		case "type", "text", "content", "cache_control":
			continue
		}
		metadata[key] = value
	}
	if len(metadata) == 0 {
		return ""
	}
	data, err := json.Marshal(sanitizeContentValue(metadata))
	if err != nil {
		return ""
	}
	return "\n[Content metadata] " + string(data) + "\n"
}
