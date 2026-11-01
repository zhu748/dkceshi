package sse

import (
	"strings"
)

func extractContentRecursive(items []any, defaultType string, thinkingEnabled bool, newType *string) ([]ContentPart, bool) {
	return extractContentRecursiveFrom(items, defaultType, thinkingEnabled, newType, "")
}

// extractContentRecursiveFrom is extractContentRecursive with the parent patch
// path available. Status ops nested under a fragment-scoped path
// (e.g. {"p":"response/fragments/-1","v":[{"p":"status","v":"FINISHED"}]}) mark
// that fragment as done, not the whole response; treating them as terminal cut
// streams short right after a web-search fragment such as "正在搜索图片".
func extractContentRecursiveFrom(items []any, defaultType string, thinkingEnabled bool, newType *string, parentPath string) ([]ContentPart, bool) {
	parts := make([]ContentPart, 0, len(items))
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		itemPath, _ := m["p"].(string)
		itemV, hasV := m["v"]
		if !hasV {
			continue
		}
		if isStatusPath(itemPath) && !isFragmentScopedPath(parentPath) {
			if s, ok := itemV.(string); ok && strings.EqualFold(strings.TrimSpace(s), "FINISHED") {
				return parts, true
			}
			continue
		}
		if shouldSkipPath(itemPath) {
			continue
		}
		if content, ok := m["content"].(string); ok && content != "" {
			typeName, _ := m["type"].(string)
			typeName = strings.ToUpper(typeName)
			switch typeName {
			case "THINK", "THINKING":
				if newType != nil && !bodyStarted(thinkingEnabled, *newType) {
					// THINK fragment moves the cursor to thinking; RESPONSE is
					// the only way back to text. Once the body has begun
					// (one-size-fits-all rule) the cursor never rewinds.
					*newType = "thinking"
				}
				parts = append(parts, ContentPart{Text: content, Type: "thinking"})
			case "RESPONSE":
				if newType != nil {
					*newType = "text"
				}
				parts = append(parts, ContentPart{Text: content, Type: "text"})
			default:
				if isToolFragmentType(typeName) {
					parts = append(parts, ContentPart{Text: content, Type: "thinking"})
				} else {
					parts = append(parts, ContentPart{Text: content, Type: defaultType})
				}
			}
			continue
		}
		partType := defaultType
		if isFragmentScopedPath(parentPath) && (itemPath == "content" || itemPath == "response/content") {
			// A bare response/fragments/-N patch (no /content suffix) updates
			// the fragment object itself. In the web-search flow these are the
			// tool fragment's status/content (e.g. "已搜索到相关图片"), which is
			// progress narration rather than answer body.
			partType = "thinking"
		} else if strings.Contains(itemPath, "thinking") {
			partType = "thinking"
		} else if itemPath == "content" || itemPath == "response" || itemPath == "fragments" || itemPath == "response/content" {
			// Explicit body/fragment paths stay text. DeepSeek also emits
			// path variants like "fragments/-1/content" (no "response/"
			// prefix) inside a response BATCH; those carry the CURRENT stream
			// kind (thinking content while reasoning, body content while
			// text), so they follow the sticky cursor instead of being forced
			// to text.
			partType = "text"
		} else if newType != nil && *newType != "" {
			partType = *newType
		}
		switch v := itemV.(type) {
		case string:
			if isStatusPath(itemPath) {
				continue
			}
			if v != "" && v != "FINISHED" {
				parts = append(parts, ContentPart{Text: v, Type: partType})
			}
		case []any:
			for _, inner := range v {
				switch x := inner.(type) {
				case map[string]any:
					innerPath, _ := x["p"].(string)
					if shouldSkipPath(innerPath) {
						continue
					}
					ct, _ := x["content"].(string)
					if ct == "" {
						// Nested fragment items may carry the payload as "v"
						// with no "content" field, e.g. a fragment content
						// APPEND inside a fragments BATCH:
						//   {"p":"-1/content","o":"APPEND","v":"..."}
						// Use the sticky cursor for its type so body code
						// (e.g. a trailing "pass\n```") is not dropped and
						// reasoning fragments are not leaked as body text.
						innerType := partType
						if newType != nil && *newType != "" {
							innerType = *newType
						}
						if sv, ok := x["v"].(string); ok && sv != "" {
							parts = append(parts, ContentPart{Text: sv, Type: innerType})
						}
						continue
					}
					typeName, _ := x["type"].(string)
					typeName = strings.ToUpper(typeName)
					switch typeName {
					case "THINK", "THINKING":
						if newType != nil && !bodyStarted(thinkingEnabled, *newType) {
							// THINK fragment moves the cursor to thinking;
							// RESPONSE is the only way back to text. Once the
							// body has begun (one-size-fits-all rule) the
							// cursor never rewinds.
							*newType = "thinking"
						}
						parts = append(parts, ContentPart{Text: ct, Type: "thinking"})
					case "RESPONSE":
						if newType != nil {
							*newType = "text"
						}
						parts = append(parts, ContentPart{Text: ct, Type: "text"})
					default:
						if isToolFragmentType(typeName) {
							parts = append(parts, ContentPart{Text: ct, Type: "thinking"})
						} else {
							parts = append(parts, ContentPart{Text: ct, Type: partType})
						}
					}
				case string:
					if x != "" {
						parts = append(parts, ContentPart{Text: x, Type: partType})
					}
				}
			}
		}
	}
	return parts, false
}
