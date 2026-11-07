package promptcompat

import (
	"ds2api/internal/prompt"
	"fmt"
	"strings"
)

// Validate before Responses items are flattened (including continuation
// restoration); normalized history cannot reveal fields discarded by adapters.
func ValidateResponsesRequestContext(request map[string]any) error {
	if value := request["instructions"]; value != nil {
		if _, ok := value.(string); !ok {
			return fmt.Errorf("instructions must be a string")
		}
	}
	if value := request["messages"]; value != nil {
		if _, ok := value.([]any); !ok {
			return fmt.Errorf("messages must be an array")
		}
	}
	if messages, ok := request["messages"].([]any); ok && len(messages) > 0 {
		if request["input"] != nil {
			return fmt.Errorf("provide input or messages, not both")
		}
		return validateHistoryShapes(messages)
	}
	var items []any
	switch input := request["input"].(type) {
	case []any:
		items = input
	case map[string]any:
		items = []any{input}
	case nil, string:
	default:
		return fmt.Errorf("input must be text, an item, or an array")
	}
	for index, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue // String/JSON compatibility items retain their own transcript.
		}
		role := strings.ToLower(strings.TrimSpace(asString(item["role"])))
		kind := strings.ToLower(strings.TrimSpace(asString(item["type"])))
		message := responsesMessageContext(item, kind)
		if message || kind == "reasoning" {
			if err := ValidateMessageMetadata(item); err != nil {
				return fmt.Errorf("input[%d]: %w", index, err)
			}
		}
		if message {
			if err := validateResponsesMessageTextAliases(item, kind); err != nil {
				return fmt.Errorf("input[%d]: %w", index, err)
			}
			if kind == "text" || kind == "input_text" || kind == "output_text" {
				if err := validatePromptMessageContent([]any{item}); err != nil {
					return fmt.Errorf("input[%d]: %w", index, err)
				}
			}
			if value := item["text"]; value != nil {
				if _, ok := value.(string); !ok {
					return fmt.Errorf("input[%d].text must be a string", index)
				}
			}
			if err := validatePromptMessageContent(item["content"]); err != nil {
				return fmt.Errorf("input[%d]: %w", index, err)
			}
		}
		if role != "" && responsesMessageItem(kind) {
			if err := validateHistoryShapes([]any{item}); err != nil {
				return fmt.Errorf("input[%d]: %w", index, err)
			}
		}
		if responsesMessageItem(kind) && (role == "tool" || role == "function") {
			if err := validateStringAliases(item["call_id"], item["tool_call_id"], "tool call ID"); err != nil {
				return fmt.Errorf("input[%d]: %w", index, err)
			}
		}
		if expected := responsesNativeItemRole(kind); expected != "" && role != "" {
			actual := role
			if actual == "function" {
				actual = "tool"
			}
			if actual != expected {
				return fmt.Errorf("input[%d] %s requires %s role", index, kind, expected)
			}
		}
		switch kind {
		case "reasoning":
			if err := validateResponsesReasoning(item); err != nil {
				return fmt.Errorf("input[%d]: %w", index, err)
			}
		case "function_call", "custom_tool_call", "tool_call":
			function, ok := item["function"].(map[string]any)
			if item["function"] != nil && !ok {
				return fmt.Errorf("input[%d].function must be an object", index)
			}
			if err := validateStringAliases(item["name"], function["name"], "tool name"); err != nil {
				return fmt.Errorf("input[%d]: %w", index, err)
			}
			if err := validateToolArgumentAliases(item, function); err != nil {
				return fmt.Errorf("input[%d]: %w", index, err)
			}
			fallthrough
		case "function_call_output", "custom_tool_call_output", "tool_result":
			if err := validateHistoryIdentityFields(item); err != nil {
				return fmt.Errorf("input[%d]: %w", index, err)
			}
			if err := validateStringAliases(item["call_id"], item["tool_call_id"], "tool call ID"); err != nil {
				return fmt.Errorf("input[%d]: %w", index, err)
			}
			if strings.HasSuffix(kind, "output") || kind == "tool_result" {
				if err := validatePayloadAliases(item, "output", "content"); err != nil {
					return fmt.Errorf("input[%d]: %w", index, err)
				}
				if err := validateStringAliases(item["name"], item["tool_name"], "tool name"); err != nil {
					return fmt.Errorf("input[%d]: %w", index, err)
				}
			}
		}
	}
	return nil
}

// Native item payloads retain their meaning when a compatibility client also
// supplies role. The role must not turn an output into a content:null message.
func responsesNativeItemRole(kind string) string {
	switch kind {
	case "function_call", "custom_tool_call", "tool_call", "reasoning":
		return "assistant"
	case "function_call_output", "custom_tool_call_output", "tool_result":
		return "tool"
	case "input_image", "image_url", "input_file", "file":
		return "user"
	}
	return ""
}

func responsesMessageItem(kind string) bool {
	switch kind {
	case "", "message", "input_message", "text", "input_text", "output_text":
		return true
	}
	return false
}

func validateStringAliases(first, second any, label string) error {
	for _, value := range []any{first, second} {
		if value != nil {
			if _, ok := value.(string); !ok {
				return fmt.Errorf("%s must be a string", label)
			}
		}
	}
	a, b := strings.TrimSpace(asString(first)), strings.TrimSpace(asString(second))
	if a != "" && b != "" && a != b {
		return fmt.Errorf("conflicting %s aliases", label)
	}
	return nil
}

func validateResponsesReasoning(item map[string]any) error {
	if value := item["text"]; value != nil {
		if _, ok := value.(string); !ok {
			return fmt.Errorf("reasoning text must be a string")
		}
	}
	for _, field := range []string{"content", "summary"} {
		if item[field] == nil {
			continue
		}
		blocks, ok := item[field].([]any)
		if !ok {
			return fmt.Errorf("reasoning %s must be an array", field)
		}
		for index, raw := range blocks {
			block, ok := raw.(map[string]any)
			if !ok {
				return fmt.Errorf("reasoning %s[%d] must be a text object", field, index)
			}
			if _, ok := block["text"].(string); !ok {
				return fmt.Errorf("reasoning %s[%d].text must be a string", field, index)
			}
			if err := prompt.ValidateTextAliases(block, "text", "content"); err != nil {
				return fmt.Errorf("reasoning %s[%d]: %w", field, index, err)
			}
		}
	}
	return nil
}
