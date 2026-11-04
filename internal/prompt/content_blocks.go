package prompt

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// NormalizeAssistantContent also preserves Chat's top-level refusal field.
// Both prompt rendering and archived request messages use this text.
func NormalizeAssistantContent(msg map[string]any) string {
	content := NormalizeContent(msg["content"])
	if refusal, _ := msg["refusal"].(string); refusal != "" {
		if content != "" {
			content += "\n\n"
		}
		content += "[refusal]" + refusal + "[/refusal]"
	}
	return content
}

// Unknown blocks remain visible as labeled data. Binary bodies and opaque
// signatures cannot meaningfully become chat text and must not flood prompts.
func unknownContentBlock(block map[string]any) string {
	data, err := json.Marshal(sanitizeContentValue(block))
	if err != nil {
		return "[Unsupported content block]"
	}
	const limit = 4096
	if len(data) > limit {
		end := limit
		for end > 0 && !utf8.RuneStart(data[end]) {
			end--
		}
		return "[Unsupported content block] " + string(data[:end]) + "...(truncated)"
	}
	return "[Unsupported content block] " + string(data)
}

func sanitizeContentValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(value))
		for key, item := range value {
			switch strings.ToLower(key) {
			case "data", "bytes", "base64", "file_data", "filedata", "audio_data", "image_data", "inline_data", "inlinedata", "encrypted_content", "signature", "thoughtsignature":
				out[key] = "[omitted_binary_payload]"
			default:
				out[key] = sanitizeContentValue(item)
			}
		}
		return out
	case []any:
		out := make([]any, len(value))
		for i, item := range value {
			out[i] = sanitizeContentValue(item)
		}
		return out
	case string:
		if strings.HasPrefix(strings.TrimSpace(value), "data:") {
			return "[omitted_binary_payload]"
		}
	}
	return value
}
