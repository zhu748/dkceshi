package promptcompat

import (
	"encoding/json"
	"strings"
)

func ResponsesMessagesFromRequest(req map[string]any) []any {
	if msgs, ok := req["messages"].([]any); ok && len(msgs) > 0 {
		return prependInstructionMessage(msgs, req["instructions"])
	}
	if rawInput, ok := req["input"]; ok {
		if msgs := NormalizeResponsesInputAsMessages(rawInput); len(msgs) > 0 {
			return prependInstructionMessage(msgs, req["instructions"])
		}
	}
	return nil
}

func prependInstructionMessage(messages []any, instructions any) []any {
	sys, _ := instructions.(string)
	if strings.TrimSpace(sys) == "" {
		return messages
	}
	out := make([]any, 0, len(messages)+1)
	out = append(out, map[string]any{"role": "system", "content": sys})
	out = append(out, messages...)
	return out
}

func NormalizeResponsesInputAsMessages(input any) []any {
	switch v := input.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return []any{map[string]any{"role": "user", "content": v}}
	case []any:
		return normalizeResponsesInputArray(v)
	case map[string]any:
		if msg := normalizeResponsesInputItem(v); msg != nil {
			return []any{msg}
		}
		if txt, _ := v["text"].(string); strings.TrimSpace(txt) != "" {
			return []any{map[string]any{"role": "user", "content": txt}}
		}
		if content, ok := v["content"]; ok {
			if strings.TrimSpace(NormalizeOpenAIContentForPrompt(content)) != "" {
				return []any{map[string]any{"role": "user", "content": content}}
			}
		}
	}
	return nil
}

func normalizeResponsesInputArray(items []any) []any {
	if len(items) == 0 {
		return nil
	}
	out := make([]any, 0, len(items))
	callNameByID := map[string]string{}
	fallbackParts := make([]string, 0, len(items))
	pendingAssistantReasoning := ""
	flushFallback := func() {
		if len(fallbackParts) == 0 {
			return
		}
		if pendingAssistantReasoning != "" {
			out = append(out, map[string]any{"role": "assistant", "reasoning_content": pendingAssistantReasoning})
			pendingAssistantReasoning = ""
		}
		out = append(out, map[string]any{"role": "user", "content": strings.Join(fallbackParts, "\n")})
		fallbackParts = fallbackParts[:0]
	}
	flushPendingReasoning := func() {
		if pendingAssistantReasoning == "" {
			return
		}
		out = append(out, map[string]any{"role": "assistant", "reasoning_content": pendingAssistantReasoning})
		pendingAssistantReasoning = ""
	}

	for _, item := range items {
		switch x := item.(type) {
		case map[string]any:
			if msg := normalizeResponsesInputItemWithState(x, callNameByID); msg != nil {
				flushFallback()
				if reasoning := assistantReasoningOnlyContent(msg); reasoning != "" {
					if pendingAssistantReasoning == "" {
						pendingAssistantReasoning = reasoning
					} else {
						pendingAssistantReasoning += "\n" + reasoning
					}
					continue
				}
				if isAssistantToolCallMessage(msg) && pendingAssistantReasoning != "" {
					reasoning := nonBlankPromptText(normalizeOpenAIReasoningContentForPrompt(msg["reasoning_content"]))
					if reasoning != "" {
						reasoning = "\n" + reasoning
					}
					msg["reasoning_content"] = pendingAssistantReasoning + reasoning
					pendingAssistantReasoning = ""
				} else {
					flushPendingReasoning()
				}
				flushFallback()
				if isAssistantToolCallMessage(msg) && len(out) > 0 {
					if merged := mergeResponsesAssistantToolCalls(out[len(out)-1], msg); merged {
						continue
					}
				}
				out = append(out, msg)
				continue
			}
			if s := normalizeResponsesFallbackPart(x); s != "" {
				flushPendingReasoning()
				fallbackParts = append(fallbackParts, s)
			}
		default:
			s, ok := item.(string)
			if !ok {
				encoded, err := json.Marshal(item)
				if err != nil {
					continue
				}
				s = string(encoded)
			}
			if strings.TrimSpace(s) != "" {
				flushPendingReasoning()
				fallbackParts = append(fallbackParts, s)
			}
		}
	}
	flushPendingReasoning()
	flushFallback()
	if len(out) == 0 {
		return nil
	}
	return out
}

func assistantReasoningOnlyContent(msg map[string]any) string {
	if !isAssistantMessage(msg) || isAssistantToolCallMessage(msg) {
		return ""
	}
	if msg["refusal"] != nil || msg["prefix"] != nil || msg["name"] != nil {
		return ""
	}
	if _, hasContent := msg["content"]; hasContent {
		content := strings.TrimSpace(NormalizeOpenAIContentForPrompt(msg["content"]))
		reasoning := nonBlankPromptText(extractOpenAIReasoningContentFromMessage(msg["content"]))
		if reasoning == "" || content != "" && content != strings.TrimSpace(reasoning) {
			return ""
		}
		return reasoning
	}
	return nonBlankPromptText(normalizeOpenAIReasoningContentForPrompt(msg["reasoning_content"]))
}

func isAssistantMessage(msg map[string]any) bool {
	return strings.EqualFold(strings.TrimSpace(asString(msg["role"])), "assistant")
}

func isAssistantToolCallMessage(msg map[string]any) bool {
	if !isAssistantMessage(msg) {
		return false
	}
	toolCalls, ok := msg["tool_calls"].([]any)
	return ok && len(toolCalls) > 0
}

func mergeResponsesAssistantToolCalls(prev any, next map[string]any) bool {
	prevMsg, ok := prev.(map[string]any)
	if !ok || !isAssistantToolCallMessage(prevMsg) || !isAssistantToolCallMessage(next) {
		return false
	}
	// Explicit message bodies, including empty bodies, retain their boundaries.
	if _, present := prevMsg["content"]; present {
		return false
	}
	if _, present := next["content"]; present {
		return false
	}
	prevCalls, _ := prevMsg["tool_calls"].([]any)
	nextCalls, _ := next["tool_calls"].([]any)
	prevMsg["tool_calls"] = append(prevCalls, nextCalls...)
	if reasoning := nonBlankPromptText(normalizeOpenAIReasoningContentForPrompt(next["reasoning_content"])); reasoning != "" {
		previous := nonBlankPromptText(normalizeOpenAIReasoningContentForPrompt(prevMsg["reasoning_content"]))
		if previous != "" {
			reasoning = previous + "\n" + reasoning
		}
		prevMsg["reasoning_content"] = reasoning
	}
	return true
}
