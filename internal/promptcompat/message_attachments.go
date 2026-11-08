package promptcompat

import "strings"

// Keep message-owned attachments beside their original body. This produces a
// fresh standard message, so rebuilding a prompt or bridging it through Chat
// never appends the same attachment twice or changes the caller's input.
func messageWithAttachmentContext(message map[string]any) map[string]any {
	var attachments []any
	for _, key := range AttachmentInputKeys() {
		attachments = appendRequestAttachments(attachments, message[key])
	}
	if len(attachments) == 0 {
		return message
	}
	out := shallowCopyMap(message)
	var content []any
	if blocks, ok := message["content"].([]any); ok {
		content = append(content, blocks...)
	} else if body := message["content"]; body != nil {
		content = append(content, map[string]any{"type": "text", "text": NormalizeOpenAIContentForPrompt(body)})
	} else if role := normalizeOpenAIRoleForPrompt(asString(message["role"])); role == "tool" || role == "function" {
		content = append(content, map[string]any{"type": "text", "text": "null"})
	}
	if normalizeOpenAIRoleForPrompt(asString(message["role"])) == "assistant" && message["prefix"] == true {
		out["content"] = append(attachments, content...)
	} else {
		out["content"] = append(content, attachments...)
	}
	for _, key := range AttachmentInputKeys() {
		delete(out, key)
	}
	return out
}

func messagesWithAttachmentContext(messages []any) []any {
	out := messages
	copied := false
	for i, raw := range messages {
		message, ok := raw.(map[string]any)
		if !ok || !hasMessageAttachments(message) {
			continue
		}
		if !copied {
			out = append([]any(nil), messages...)
			copied = true
		}
		out[i] = messageWithAttachmentContext(message)
	}
	return out
}

func hasMessageAttachments(message map[string]any) bool {
	for _, key := range AttachmentInputKeys() {
		switch value := message[key].(type) {
		case nil:
			continue
		case string:
			if strings.TrimSpace(value) == "" {
				continue
			}
		case []any:
			if len(value) == 0 {
				continue
			}
		case []string:
			if len(value) == 0 {
				continue
			}
		}
		return true
	}
	return false
}

func copyMessageAttachments(out, source map[string]any) {
	for _, key := range AttachmentInputKeys() {
		if value, present := source[key]; present {
			out[key] = value
		}
	}
}

// AppendMessageAttachments keeps native tool-item attachments when the adapter
// turns its payload into a standard assistant/tool message. Distinct native
// containers remain in source order; source slices are never modified.
func AppendMessageAttachments(out map[string]any, sources ...map[string]any) {
	var appended []any
	for _, source := range sources {
		for _, key := range AttachmentInputKeys() {
			appended = appendRequestAttachments(appended, source[key])
		}
	}
	if len(appended) == 0 {
		return
	}
	var existing []any
	for _, key := range AttachmentInputKeys() {
		existing = appendRequestAttachments(existing, out[key])
		delete(out, key)
	}
	out["attachments"] = append(existing, appended...)
}

func validateMessageAttachments(message map[string]any) error {
	for _, key := range AttachmentInputKeys() {
		if err := validatePromptMessageContent(appendRequestAttachments(nil, message[key])); err != nil {
			return err
		}
	}
	return nil
}
