package prompt

import (
	"fmt"
	"strings"
)

// ResolveFileReferenceID compares every supplied ID spelling of one attachment.
// Byte/URL hints are independent and may be stale when a valid ID is supplied.
// Callers establish attachment identity before calling this on business JSON.
func ResolveFileReferenceID(block map[string]any) (string, error) {
	kind, _ := block["type"].(string)
	kind = strings.ToLower(strings.TrimSpace(kind))
	fileKind := kind == "file" || kind == "input_file" || kind == "file_reference"
	id := ""
	for depth, current := 0, block; current != nil; depth++ {
		if depth >= 128 {
			return "", fmt.Errorf("file reference nesting exceeds the limit")
		}
		keys := []string{"file_id"}
		if fileKind || depth > 0 {
			keys = append(keys, "id")
		}
		for _, key := range keys {
			value := current[key]
			if value == nil {
				continue
			}
			text, ok := value.(string)
			if !ok {
				return "", fmt.Errorf("file reference %s must be a string", key)
			}
			text = strings.TrimSpace(text)
			if text == "" {
				continue
			}
			if id != "" && id != text {
				return "", fmt.Errorf("conflicting file reference ID aliases")
			}
			id = text
		}
		current, _ = current["file"].(map[string]any)
	}
	return id, nil
}
