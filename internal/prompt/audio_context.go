package prompt

import (
	"encoding/json"
	"strings"
)

// Audio is a reference/transcript envelope, not an input audio decoder. Preserve
// its textual extensions and mark only payload/state fields as omitted data.
func audioContextText(raw any) string {
	audio, ok := raw.(map[string]any)
	if !ok {
		return ""
	}
	var text string
	if transcript, ok := audio["transcript"].(string); ok {
		text += "\n[audio_transcript]" + transcript + "[/audio_transcript]"
	}
	if id, ok := audio["id"].(string); ok && id != "" {
		encoded, _ := json.Marshal(id)
		text += "\n[Audio reference] " + string(encoded)
	}
	fields := sanitizedAudioContext(audio)
	if _, consumed := audio["transcript"].(string); consumed {
		delete(fields, "transcript")
	}
	if id, consumed := audio["id"].(string); consumed && id != "" {
		delete(fields, "id")
	}
	if len(fields) > 0 {
		encoded, _ := json.Marshal(fields)
		text += "\n[Audio context (JSON data)] " + string(encoded)
	}
	return text
}

func sanitizedAudioContext(audio map[string]any) map[string]any {
	fields := make(map[string]any, len(audio))
	for key, value := range audio {
		switch strings.ToLower(key) {
		case "cache_control":
			continue
		case "data", "bytes", "base64", "audio_data", "file_data", "filedata", "inline_data", "inlinedata", "signature", "encrypted_content", "thoughtsignature", "thought_signature":
			fields[key] = "[omitted_binary_payload]"
		default:
			fields[key] = sanitizeContentValue(value)
		}
	}
	return fields
}

func AudioContextOmittedPaths(raw any) []string {
	audio, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	var paths []string
	collectOmittedPaths(audio, sanitizedAudioContext(audio), "", &paths)
	return paths
}
