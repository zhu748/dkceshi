package prompt

import "strings"

// PreserveTextContextBlock recognizes documented textual context before the
// bounded unknown-block fallback. Binary fields remain scrubbed; known text,
// document and search-result bodies never use binary heuristics or truncation.
func PreserveTextContextBlock(block map[string]any) (map[string]any, bool) {
	field := textContextField(block)
	if field == "" {
		return nil, false
	}
	out := sanitizeContentValue(block).(map[string]any)
	switch field {
	case "text":
		out[field] = block[field]
	case "content":
		out[field] = preserveTextContextParts(block[field])
	case "source":
		source := block[field].(map[string]any)
		copy := out[field].(map[string]any)
		if source["type"] == "text" {
			if text, ok := source["data"].(string); ok {
				copy["data"] = text
			}
		} else {
			copy["content"] = preserveTextContextParts(source["content"])
		}
	}
	return out, true
}

// IsTextContextBlock classifies without sanitizing/copying uploadable children.
// Adapters keep those source children until shared file processing runs.
func IsTextContextBlock(block map[string]any) bool { return textContextField(block) != "" }

func textContextField(block map[string]any) string {
	kind, _ := block["type"].(string)
	kind = strings.ToLower(strings.TrimSpace(kind))
	switch kind {
	case "text", "input_text", "output_text":
		if _, ok := block["text"].(string); !ok {
			return ""
		}
		return "text"
	case "search_result":
		if _, ok := block["content"].([]any); !ok {
			return ""
		}
		return "content"
	case "document":
		source, _ := block["source"].(map[string]any)
		if source["type"] != "text" && source["type"] != "content" {
			return ""
		}
		return "source"
	default:
		return ""
	}
}

func preserveTextContextParts(raw any) any {
	parts, ok := raw.([]any)
	if !ok {
		return sanitizeContentValue(raw)
	}
	out := make([]any, len(parts))
	for i, part := range parts {
		if block, ok := part.(map[string]any); ok {
			if preserved, ok := PreserveTextContextBlock(block); ok {
				out[i] = preserved
				continue
			}
		}
		out[i] = sanitizeContentValue(part)
	}
	return out
}
