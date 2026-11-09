package promptcompat

import (
	"fmt"
	"strings"

	"ds2api/internal/config"
	"ds2api/internal/prompt"
)

func normalizeResponsesInputItem(m map[string]any) map[string]any {
	return normalizeResponsesInputItemWithState(m, nil)
}

func normalizeResponsesInputItemWithState(m map[string]any, callNameByID map[string]string) map[string]any {
	if m == nil {
		return nil
	}

	itemType := strings.ToLower(strings.TrimSpace(asString(m["type"])))
	role := strings.ToLower(strings.TrimSpace(asString(m["role"])))
	if text := normalizeResponsesTextItem(m, itemType, role); text != nil {
		return text
	}
	if responsesMessageContext(m, itemType) {
		if role == "" {
			role = "user"
		}
		if role == "assistant" {
			return normalizeResponsesAssistantMessage(m)
		}
		content, _ := responsesMessageBody(m)
		out := map[string]any{
			"role":    normalizeOpenAIRoleForPrompt(role),
			"content": content,
		}
		copyPromptMessageMetadata(out, m)
		copyMessageContextFields(out, m, "type", "text", "call_id")
		if role == "tool" || role == "function" {
			if value, ok := m["is_error"]; ok {
				out["is_error"] = value
			}
			if callID := strings.TrimSpace(asString(m["tool_call_id"])); callID != "" {
				out["tool_call_id"] = callID
			}
			if callID := strings.TrimSpace(asString(m["call_id"])); callID != "" {
				out["tool_call_id"] = callID
			}
			if name := strings.TrimSpace(asString(m["name"])); name != "" {
				out["name"] = name
			}
		}
		return out
	}

	switch itemType {
	case "input_image", "image_url", "input_file", "file":
		return map[string]any{"role": "user", "content": []any{m}}
	case "function_call_output", "custom_tool_call_output", "tool_result":
		content, hasOutput := m["output"]
		if !hasOutput {
			content = m["content"]
		}
		out := map[string]any{
			"role":    "tool",
			"content": content,
		}
		copyPromptMessageMetadata(out, m)
		copyMessageContextFields(out, m, "type", "output", "call_id", "tool_name")
		if value, ok := m["is_error"]; ok {
			out["is_error"] = value
		}
		if callID := strings.TrimSpace(asString(m["call_id"])); callID != "" {
			out["tool_call_id"] = callID
		} else if callID = strings.TrimSpace(asString(m["tool_call_id"])); callID != "" {
			out["tool_call_id"] = callID
		}
		if name := strings.TrimSpace(asString(m["name"])); name != "" {
			out["name"] = name
		} else if name = strings.TrimSpace(asString(m["tool_name"])); name != "" {
			out["name"] = name
		} else if callID := strings.TrimSpace(asString(out["tool_call_id"])); callID != "" {
			if inferred := strings.TrimSpace(callNameByID[callID]); inferred != "" {
				out["name"] = inferred
			} else {
				config.Logger.Warn(
					"[responses] unable to backfill tool result name from call_id",
					"call_id", callID,
				)
			}
		}
		return out
	case "function_call", "custom_tool_call", "tool_call":
		name := strings.TrimSpace(asString(m["name"]))
		var fn map[string]any
		if rawFn, ok := m["function"].(map[string]any); ok {
			fn = rawFn
			if name == "" {
				name = strings.TrimSpace(asString(fn["name"]))
			}
		}
		argsRaw, supplied := prompt.ToolCallArguments(m)

		functionPayload := shallowCopyMap(fn)
		delete(functionPayload, "input") // Consumed argument alias; retain extensions.
		functionPayload["name"] = name
		functionPayload["arguments"] = prompt.StringifyToolCallArguments(argsRaw)
		if _, text := argsRaw.(string); supplied && (text || argsRaw == nil) {
			functionPayload["arguments"] = argsRaw
		}
		if itemType == "custom_tool_call" {
			functionPayload["arguments"] = map[string]any{"input": argsRaw}
		}
		call := map[string]any{
			"type":     "function",
			"function": functionPayload,
		}
		// A Responses item ID and its call ID identify different things. Keep
		// item identity with this occurrence even when adjacent calls merge.
		if value, present := m["id"]; present {
			call["responses_item_id"] = value
		}
		if callID := strings.TrimSpace(asString(m["call_id"])); callID != "" {
			call["id"] = callID
		} else if callID = strings.TrimSpace(asString(m["tool_call_id"])); callID != "" {
			call["id"] = callID
		} else if callID = strings.TrimSpace(asString(m["id"])); callID != "" {
			call["id"] = callID
		}
		if callID := strings.TrimSpace(asString(call["id"])); callID != "" && callNameByID != nil {
			callNameByID[callID] = name
		}
		out := map[string]any{
			"role":       "assistant",
			"tool_calls": []any{call},
		}
		copyMessageAttachments(out, m)
		copyMessageContextFields(out, m, "type", "arguments", "input", "function", "call_id")
		return out
	case "reasoning":
		text := responsesReasoningText(m)
		return map[string]any{"role": "assistant", "reasoning_content": text, "id": m["id"]}
	}

	if itemType != "" {
		if role == "" {
			role = "user"
			if strings.HasSuffix(itemType, "_call") || strings.HasSuffix(itemType, "_call_output") || strings.HasPrefix(itemType, "mcp_") {
				role = "assistant"
			}
		}
		// Built-in tool records are historical context, never executable calls.
		// Keep their complete typed data rather than extracting only text/content.
		return map[string]any{"role": role, "content": []any{m}}
	}
	return map[string]any{"role": "user", "content": []any{m}}
}

func normalizeResponsesAssistantMessage(m map[string]any) map[string]any {
	out := map[string]any{
		"role": "assistant",
	}
	copyMessageAttachments(out, m)
	copyMessageContextFields(out, m, "type", "text")
	for _, key := range []string{"refusal", "id", "prefix", "name", "audio"} {
		if value, exists := m[key]; exists {
			out[key] = value
		}
	}
	if call, exists := m["function_call"]; exists {
		out["function_call"] = call
		out["id"] = m["id"]
	}
	if toolCalls, exists := m["tool_calls"]; exists {
		out["tool_calls"] = toolCalls
	}
	content, hasContent := responsesMessageBody(m)
	if hasContent {
		out["content"] = content
	}
	if reasoning := normalizeOpenAIReasoningContentForPrompt(m["reasoning_content"]); reasoning != "" {
		out["reasoning_content"] = m["reasoning_content"]
	}
	if _, hasToolCalls := out["tool_calls"]; hasToolCalls || out["function_call"] != nil || hasContent || out["reasoning_content"] != nil || hasPromptMessageMetadata(out) {
		return out
	}
	return nil
}

func normalizeResponsesFallbackPart(m map[string]any) string {
	if m == nil {
		return ""
	}
	if t, _ := m["type"].(string); strings.EqualFold(strings.TrimSpace(t), "input_text") {
		if txt, _ := m["text"].(string); strings.TrimSpace(txt) != "" {
			return txt
		}
	}
	if txt, _ := m["text"].(string); strings.TrimSpace(txt) != "" {
		return txt
	}
	if content, ok := m["content"]; ok {
		if normalized := strings.TrimSpace(NormalizeOpenAIContentForPrompt(content)); normalized != "" {
			return normalized
		}
	}
	if m["type"] != nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", m))
}

func responsesReasoningText(item map[string]any) string {
	var sources []string
	for _, key := range []string{"content", "summary"} {
		var parts []string
		if entries, ok := item[key].([]any); ok {
			for _, raw := range entries {
				entry, _ := raw.(map[string]any)
				if text, ok := entry["text"].(string); ok {
					parts = append(parts, text+prompt.TextBlockMetadata(entry))
				}
			}
		}
		if len(parts) > 0 {
			sources = append(sources, strings.Join(parts, "\n"))
		}
	}
	sources = append(sources, asString(item["text"]))
	return prompt.JoinReasoningRepresentations(sources...) + responsesReasoningMetadata(item)
}
