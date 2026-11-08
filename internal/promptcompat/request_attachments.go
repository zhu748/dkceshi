package promptcompat

import "strings"

// Include request-level attachment context in the current user task. The
// caller's messages and blocks remain unchanged so preprocessing and replay
// can rebuild the same normalized request repeatedly.
func WithRequestAttachmentContext(messages []any, request map[string]any) ([]any, bool) {
	messages = messagesWithAttachmentContext(messages)
	var attachments []any
	for _, key := range AttachmentInputKeys() {
		attachments = appendRequestAttachments(attachments, request[key])
	}
	if len(attachments) == 0 {
		return messages, false
	}
	out := append([]any(nil), messages...)
	for i := len(out) - 1; i >= 0; i-- {
		message, _ := out[i].(map[string]any)
		if !strings.EqualFold(strings.TrimSpace(asString(message["role"])), "user") {
			continue
		}
		copy := make(map[string]any, len(message))
		for key, value := range message {
			copy[key] = value
		}
		var content []any
		if blocks, ok := message["content"].([]any); ok {
			content = append(content, blocks...)
		} else if message["content"] != nil {
			content = append(content, map[string]any{"type": "text", "text": NormalizeOpenAIContentForPrompt(message["content"])})
		}
		copy["content"] = append(content, attachments...)
		out[i] = copy
		return out, true
	}
	// Partial histories can start with an assistant/tool loop. Place its
	// request-wide attachments before that loop, preserving any final prefill.
	index := 0
	for index < len(out) {
		message, _ := out[index].(map[string]any)
		role := strings.ToLower(strings.TrimSpace(asString(message["role"])))
		if role != "system" && role != "developer" {
			break
		}
		index++
	}
	message := map[string]any{"role": "user", "content": attachments}
	out = append(out, nil)
	copy(out[index+1:], out[index:])
	out[index] = message
	return out, true
}

func appendRequestAttachments(out []any, raw any) []any {
	switch value := raw.(type) {
	case nil:
		return out
	case []any:
		for _, item := range value {
			out = appendRequestAttachments(out, item)
		}
		return out
	case []string:
		for _, item := range value {
			out = appendRequestAttachments(out, item)
		}
		return out
	case string:
		if id := strings.TrimSpace(value); id != "" {
			out = append(out, map[string]any{"type": "input_file", "file_id": id})
		}
		return out
	case map[string]any:
		if value["type"] == nil && (value["file_id"] != nil || value["file"] != nil || value["file_data"] != nil || value["image_url"] != nil) {
			block := make(map[string]any, len(value)+1)
			for key, item := range value {
				block[key] = item
			}
			block["type"] = "input_file"
			if value["image_url"] != nil {
				block["type"] = "image_url"
			}
			return append(out, block)
		}
	}
	return append(out, raw)
}

// Top-level attachment arrays hold content blocks, not message envelopes.
func stripRequestAttachmentImages(request map[string]any) {
	for _, key := range AttachmentInputKeys() {
		raw, present := request[key]
		if !present {
			continue
		}
		if block, ok := raw.(map[string]any); ok {
			raw = []any{block}
		}
		request[key] = stripImageContent(raw)
	}
}
