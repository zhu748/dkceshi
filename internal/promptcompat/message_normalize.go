package promptcompat

import (
	"strings"

	"ds2api/internal/prompt"
	"ds2api/internal/toolcall"
)

const assistantReasoningLabel = "reasoning_content"

// NormalizeOpenAIMessagesForPrompt preserves message boundaries and metadata.
// Generated history tool tags use the caller marker; ordinary content is never
// rewritten by a later whole-prompt substitution.
func NormalizeOpenAIMessagesForPrompt(raw []any, traceID string, toolMarker ...string) []map[string]any {
	_ = traceID
	raw = NormalizeLegacyToolHistory(raw)
	marker := firstToolMarker(toolMarker)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		msg, ok := item.(map[string]any)
		if !ok {
			continue
		}
		msg = messageWithAttachmentContext(msg)
		before := len(out)
		role := strings.ToLower(strings.TrimSpace(asString(msg["role"])))
		switch role {
		case "assistant":
			content := buildAssistantContentForPrompt(msg, marker)
			if content == "" && !hasExplicitPromptMessage(msg) {
				continue
			}
			out = append(out, map[string]any{
				"role":    "assistant",
				"content": content,
				"prefix":  msg["prefix"],
			})
		case "tool", "function":
			content := buildToolContentForPrompt(msg)
			out = append(out, map[string]any{
				"role":         "tool",
				"content":      content,
				"tool_call_id": msg["tool_call_id"],
				"name":         msg["name"],
				"is_error":     msg["is_error"],
			})
		case "user", "system", "developer":
			out = append(out, map[string]any{
				"role":    normalizeOpenAIRoleForPrompt(role),
				"content": NormalizeOpenAIContentForPrompt(msg["content"]),
			})
		default:
			content := NormalizeOpenAIContentForPrompt(msg["content"])
			if content == "" {
				continue
			}
			if role == "" {
				role = "user"
			}
			out = append(out, map[string]any{
				"role":    normalizeOpenAIRoleForPrompt(role),
				"content": content,
			})
		}
		if len(out) > before {
			for _, key := range []string{"name", "id", "prefix", "tool_call_id", "is_error"} {
				if value, ok := msg[key]; ok {
					out[len(out)-1][key] = value
				}
			}
		}
	}
	return out
}

func buildAssistantContentForPrompt(msg map[string]any, marker string) string {
	toolHistory := prompt.FormatToolCallsForPromptForMarker(msg["tool_calls"], marker)
	content, reasoning := assistantContentForPrompt(msg, marker, toolHistory == "")
	parts := make([]string, 0, 3)
	if reasoning != "" {
		parts = append(parts, formatPromptLabeledBlock(assistantReasoningLabel, reasoning))
	}
	if content != "" {
		parts = append(parts, content)
	}
	if toolHistory != "" {
		parts = append(parts, toolHistory)
	}
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	default:
		return strings.Join(parts, "\n\n")
	}
}

// normalizeAssistantToolMarkupContentForPrompt re-renders an assistant message
// whose plain content carries a standalone tool-call block into the canonical
// EPSE history shell, so a malformed-but-parseable block does not keep
// polluting later turns as ad-hoc assistant prose.
//
// The history may carry either the canonical EPSE form or the caller-specific
// marker form; the marker is normalized back to EPSE before the block probe and
// parse so both forms are recognized. Non-tool content is returned unchanged
// (including marker-form text that does not parse, which already is the form the
// prompt boundary wants).
func normalizeAssistantToolMarkupContentForPrompt(content, marker string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return content
	}
	trimmed = toolcall.NormalizeToolMarkupMarker(trimmed, marker)
	if !isStandaloneAssistantToolMarkupBlock(trimmed) {
		return content
	}
	parsed := toolcall.ParseStandaloneToolCallsDetailed(trimmed, nil)
	if len(parsed.Calls) == 0 || parsed.SawToolCallIntent || strings.TrimSpace(parsed.ResidualProtocolText) != "" {
		return content
	}
	parsed.Calls = toolcall.NormalizeParsedToolCallsForSchemas(parsed.Calls, nil)
	raw := make([]any, 0, len(parsed.Calls))
	for _, call := range parsed.Calls {
		raw = append(raw, map[string]any{
			"id":    call.CallID,
			"name":  call.Name,
			"input": call.Input,
		})
	}
	if formatted := prompt.FormatToolCallsForPromptForMarker(raw, marker); formatted != "" {
		return formatted
	}
	return content
}

func isStandaloneAssistantToolMarkupBlock(trimmed string) bool {
	tag, ok := toolcall.FindToolMarkupTagOutsideIgnored(trimmed, 0)
	if !ok || tag.Start != 0 || tag.Closing || tag.Name != "tool_calls" {
		return false
	}
	closeTag, ok := toolcall.FindMatchingToolMarkupClose(trimmed, tag)
	if !ok {
		return false
	}
	return strings.TrimSpace(trimmed[closeTag.End+1:]) == ""
}

func normalizeOpenAIReasoningContentForPrompt(v any) string {
	return prompt.ReasoningText(v)
}

func extractOpenAIReasoningContentFromMessage(v any) string {
	return prompt.ContentReasoningText(v)
}

func formatPromptLabeledBlock(label, text string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return text
	}
	return "[" + label + "]\n" + text + "\n[/" + label + "]"
}

// Use trimming only to detect empty content, never to rewrite client text.
func nonBlankPromptText(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	return text
}

func buildToolContentForPrompt(msg map[string]any) string {
	content := NormalizeOpenAIContentForPrompt(msg["content"])
	if msg["content"] == nil {
		return "null"
	}
	return content
}

func NormalizeOpenAIContentForPrompt(v any) string {
	return prompt.NormalizeContent(v)
}

func normalizeOpenAIRoleForPrompt(role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	return role
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
