package prompt

import (
	"encoding/json"
	"strings"
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
	if audio, ok := msg["audio"].(map[string]any); ok {
		if transcript, ok := audio["transcript"].(string); ok {
			content += "\n[audio_transcript]" + transcript + "[/audio_transcript]"
		}
		if id, ok := audio["id"].(string); ok && id != "" {
			encoded, _ := json.Marshal(id)
			content += "\n[Audio reference] " + string(encoded)
		}
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
	return "[Unsupported content block] " + string(data)
}

// SanitizeContextValue retains textual context at every depth. Request size
// limits and context-file transport bound input; block serializers never trim it.
func SanitizeContextValue(value any) any { return sanitizeContentValue(value) }

func sanitizeContentValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(value))
		for key, item := range value {
			switch strings.ToLower(key) {
			case "base64", "file_data", "filedata", "audio_data", "image_data", "inline_data", "inlinedata", "encrypted_content", "signature", "thoughtsignature":
				out[key] = "[omitted_binary_payload]"
			case "data", "bytes":
				if binaryContextObject(value) {
					out[key] = "[omitted_binary_payload]"
				} else {
					out[key] = sanitizeContentValue(item)
				}
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
		if text := strings.TrimSpace(value); strings.HasPrefix(text, "data:") && strings.Contains(text, ";base64,") {
			return "[omitted_binary_payload]"
		}
	}
	return value
}

func binaryContextObject(block map[string]any) bool {
	kind, _ := block["type"].(string)
	encoding, _ := block["encoding"].(string)
	switch strings.ToLower(kind) {
	case "base64", "image", "audio", "video", "input_audio", "input_image", "input_video", "redacted_thinking":
		return true
	}
	return strings.EqualFold(encoding, "base64")
}
