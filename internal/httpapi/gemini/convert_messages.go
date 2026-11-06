package gemini

import (
	"ds2api/internal/prompt"
	"ds2api/internal/promptcompat"
	"strings"
)

func geminiMessagesFromRequest(req map[string]any) []any {
	out := make([]any, 0, 8)
	links := promptcompat.NewToolCallLinker("call_gemini")
	if sys := normalizeGeminiSystemInstruction(req["systemInstruction"]); strings.TrimSpace(sys) != "" {
		out = append(out, map[string]any{
			"role":    "system",
			"content": sys,
		})
	}

	contents, _ := req["contents"].([]any)
	for _, raw := range contents {
		content, _ := raw.(map[string]any)
		parts, _ := content["parts"].([]any)
		for _, rawPart := range parts {
			part, _ := rawPart.(map[string]any)
			for _, kind := range []string{"functionCall", "functionResponse"} {
				fn, _ := part[kind].(map[string]any)
				for _, key := range []string{"id", "call_id", "callId", "tool_call_id"} {
					links.Reserve(asString(fn[key]))
				}
			}
		}
	}
	for _, item := range contents {
		content, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role := mapGeminiRole(content["role"])
		if role == "" {
			role = "user"
		}
		parts, _ := content["parts"].([]any)
		if len(parts) == 0 {
			if text, ok := content["text"].(string); ok {
				out = append(out, map[string]any{
					"role":    role,
					"content": text,
				})
			}
			continue
		}

		textParts := make([]any, 0, len(parts))
		pendingThinking := ""
		flushText := func() {
			if len(textParts) == 0 {
				return
			}
			msg := map[string]any{
				"role":    role,
				"content": promptcompat.JoinContentText(textParts),
			}
			if role == "assistant" && strings.TrimSpace(pendingThinking) != "" {
				msg["reasoning_content"] = pendingThinking
				pendingThinking = ""
			}
			out = append(out, msg)
			textParts = textParts[:0]
		}

		for _, rawPart := range parts {
			part, ok := rawPart.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := part["text"].(string); ok {
				if role == "assistant" && isGeminiThoughtPart(part) {
					if pendingThinking == "" {
						pendingThinking = text
					} else {
						pendingThinking += "\n" + text
					}
					continue
				}
				textParts = append(textParts, geminiTextBlock(part, text))
				continue
			}

			if fnCall, ok := part["functionCall"].(map[string]any); ok {
				flushText()
				name := strings.TrimSpace(asString(fnCall["name"]))
				callID := geminiFunctionCallID(fnCall)
				callID = links.Call(name, callID)
				msg := map[string]any{
					"role": "assistant",
					"tool_calls": []any{
						map[string]any{
							"id":   callID,
							"type": "function",
							"function": map[string]any{
								"name":      name,
								"arguments": stringifyJSON(fnCall["args"]),
							},
						},
					},
				}
				if strings.TrimSpace(pendingThinking) != "" {
					msg["reasoning_content"] = pendingThinking
					pendingThinking = ""
				}
				out = append(out, msg)
				continue
			}

			if fnResp, ok := part["functionResponse"].(map[string]any); ok {
				flushText()
				name := strings.TrimSpace(asString(fnResp["name"]))
				callID := geminiFunctionCallID(fnResp)
				callID, name = links.Result(name, callID)
				msg := map[string]any{
					"role":         "tool",
					"tool_call_id": callID,
					"content":      geminiFunctionResponseContent(fnResp),
				}
				if name != "" {
					msg["name"] = name
				}
				out = append(out, msg)
				continue
			}

			if attachment, ok := promptcompat.NormalizeAttachmentContent(part); ok {
				textParts = append(textParts, attachment)
			} else if raw := strings.TrimSpace(formatGeminiUnknownPartForPrompt(part)); raw != "" && raw != "null" {
				textParts = append(textParts, raw)
			}
		}
		flushText()
		if role == "assistant" && strings.TrimSpace(pendingThinking) != "" {
			out = append(out, map[string]any{
				"role":              "assistant",
				"reasoning_content": pendingThinking,
			})
		}
	}
	return out
}

func isGeminiThoughtPart(part map[string]any) bool {
	if part == nil {
		return false
	}
	if v, ok := part["thought"].(bool); ok {
		return v
	}
	return false
}

func normalizeGeminiSystemInstruction(raw any) string {
	switch v := raw.(type) {
	case string:
		return v
	case map[string]any:
		if parts, ok := v["parts"].([]any); ok {
			texts := make([]string, 0, len(parts))
			for _, item := range parts {
				part, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if text, ok := part["text"].(string); ok {
					texts = append(texts, prompt.NormalizeContent([]any{geminiTextBlock(part, text)}))
				}
			}
			return strings.Join(texts, "")
		}
		if text, ok := v["text"].(string); ok {
			return text
		}
	}
	return ""
}

func mapGeminiRole(v any) string {
	switch strings.ToLower(strings.TrimSpace(asString(v))) {
	case "user":
		return "user"
	case "model", "assistant":
		return "assistant"
	case "system":
		return "system"
	default:
		return ""
	}
}

func formatGeminiUnknownPartForPrompt(part map[string]any) string {
	return stringifyJSON(sanitizeGeminiPartForPrompt(part))
}

func sanitizeGeminiPartForPrompt(part map[string]any) map[string]any {
	return prompt.SanitizeContextValue(part).(map[string]any)
}
