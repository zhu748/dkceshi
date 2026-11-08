package promptcompat

import "strings"

// ContentHasAttachments uses the same structural boundary as upload/reference
// discovery. Native adapters use it before any text-only sanitization can erase
// a nested attachment. Business tool arguments/results stay opaque.
func ContentHasAttachments(raw any) bool {
	type entry struct {
		value    any
		allowIDs bool
		index    int
	}
	pending := []entry{{value: raw}}
	for len(pending) > 0 {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		switch value := current.value.(type) {
		case string:
			if current.allowIDs && strings.TrimSpace(value) != "" {
				return true
			}
		case []string:
			if current.allowIDs {
				for _, id := range value {
					if strings.TrimSpace(id) != "" {
						return true
					}
				}
			}
		case []any:
			if i := current.index; i < len(value) {
				pending = append(pending, entry{value, current.allowIDs, i + 1}, entry{value[i], current.allowIDs, 0})
			}
		case map[string]any:
			if IsInlineAttachmentBlock(value) || current.allowIDs && (asString(value["file_id"]) != "" || asString(value["id"]) != "") {
				return true
			}
			for _, key := range FileInputChildKeys(value) {
				if child, present := value[key]; present {
					pending = append(pending, entry{value: child, allowIDs: referenceIDContainer(key) || current.allowIDs && key == "file"})
				}
			}
		}
	}
	return false
}
