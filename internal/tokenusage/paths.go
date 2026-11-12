package tokenusage

import "strings"

// IsMetadataPath also covers explicit usage patches, including string counts,
// so a valid count never becomes answer text.
func IsMetadataPath(path string) bool {
	path = strings.TrimPrefix(strings.Trim(path, "/"), "response/")
	first, _, _ := strings.Cut(path, "/")
	switch first {
	case "usage", "token_usage", "accumulated_token_usage", "input_tokens", "prompt_tokens", "output_tokens", "completion_tokens", "reasoning_tokens", "total_tokens", "completion_tokens_details", "output_tokens_details":
		return true
	}
	return false
}
