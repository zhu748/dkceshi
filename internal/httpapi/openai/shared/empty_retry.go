package shared

import (
	"strings"

	"ds2api/internal/promptcompat"
)

const EmptyOutputRetrySuffix = "Previous reply had no visible output. Please regenerate the visible final answer or tool call now."

func EmptyOutputRetryEnabled() bool {
	return true
}

func EmptyOutputRetryMaxAttempts() int {
	return 1
}

func ClonePayloadWithEmptyOutputRetryPrompt(payload any) any {
	return ClonePayloadForEmptyOutputRetry(payload, 0)
}

// ClonePayloadForEmptyOutputRetry creates a retry payload with the suffix
// appended and, if parentMessageID > 0, sets parent_message_id so the
// retry is submitted as a proper follow-up turn in the same DeepSeek
// session rather than a disconnected root message.
//
// 支持 *promptcompat.OrderedJSONMap 与 map[string]any 两种载体；
// OrderedJSONMap 会保留原始字段顺序，对齐真实 Android App 抓包顺序。
func ClonePayloadForEmptyOutputRetry(payload any, parentMessageID int) any {
	if m, ok := payload.(*promptcompat.OrderedJSONMap); ok {
		clone := promptcompat.NewOrderedJSONMap()
		for _, k := range m.Order {
			clone.Set(k, m.M[k])
		}
		original, _ := m.M["prompt"].(string)
		clone.Set("prompt", AppendEmptyOutputRetrySuffix(original))
		if parentMessageID > 0 {
			clone.Set("parent_message_id", parentMessageID)
		}
		return clone
	}
	if mm, ok := payload.(map[string]any); ok {
		clone := make(map[string]any, len(mm))
		for k, v := range mm {
			clone[k] = v
		}
		original, _ := mm["prompt"].(string)
		clone["prompt"] = AppendEmptyOutputRetrySuffix(original)
		if parentMessageID > 0 {
			clone["parent_message_id"] = parentMessageID
		}
		return clone
	}
	// fallback：返回原值，调用方需自行处理
	return payload
}

func AppendEmptyOutputRetrySuffix(prompt string) string {
	prompt = strings.TrimRight(prompt, "\r\n\t ")
	if prompt == "" {
		return EmptyOutputRetrySuffix
	}
	return prompt + "\n\n" + EmptyOutputRetrySuffix
}

func UsagePromptWithEmptyOutputRetry(originalPrompt string, retryAttempts int) string {
	if retryAttempts <= 0 {
		return originalPrompt
	}
	parts := make([]string, 0, retryAttempts+1)
	parts = append(parts, originalPrompt)
	next := originalPrompt
	for i := 0; i < retryAttempts; i++ {
		next = AppendEmptyOutputRetrySuffix(next)
		parts = append(parts, next)
	}
	return strings.Join(parts, "\n")
}
