package completionruntime

import "strings"

const EmptyOutputRetrySuffix = "Previous reply had no visible output. Please regenerate the visible final answer or tool call now."

const ToolOutputRetrySuffix = "Previous reply contained an invalid tool call. Regenerate the answer using only the declared tools, valid complete JSON arguments matching their schemas, and the requested tool_choice and parallel_tool_calls policy."

func ClonePayloadForToolOutputRetry(payload map[string]any, parentMessageID int) map[string]any {
	clone := ClonePayloadForEmptyOutputRetry(payload, parentMessageID)
	original, _ := payload["prompt"].(string)
	clone["prompt"] = strings.TrimRight(original, "\r\n\t ") + "\n\n" + ToolOutputRetrySuffix
	return clone
}

func EmptyOutputRetryEnabled() bool {
	return true
}

func EmptyOutputRetryMaxAttempts() int {
	return 1
}

func ClonePayloadWithEmptyOutputRetryPrompt(payload map[string]any) map[string]any {
	return ClonePayloadForEmptyOutputRetry(payload, 0)
}

// ClonePayloadForEmptyOutputRetry creates a retry payload with the suffix
// appended and, if parentMessageID > 0, sets parent_message_id so the
// retry is submitted as a proper follow-up turn in the same DeepSeek
// session rather than a disconnected root message.
func ClonePayloadForEmptyOutputRetry(payload map[string]any, parentMessageID int) map[string]any {
	clone := make(map[string]any, len(payload))
	for k, v := range payload {
		clone[k] = v
	}
	original, _ := payload["prompt"].(string)
	clone["prompt"] = AppendEmptyOutputRetrySuffix(original)
	if parentMessageID > 0 {
		clone["parent_message_id"] = parentMessageID
	}
	return clone
}

func AppendEmptyOutputRetrySuffix(prompt string) string {
	prompt = strings.TrimRight(prompt, "\r\n\t ")
	if prompt == "" {
		return EmptyOutputRetrySuffix
	}
	return prompt + "\n\n" + EmptyOutputRetrySuffix
}

func UsagePromptWithEmptyOutputRetry(originalPrompt string, retryAttempts int) string {
	return usagePromptWithRetrySuffix(originalPrompt, retryAttempts, EmptyOutputRetrySuffix)
}

func UsagePromptWithToolOutputRetry(originalPrompt string, retryAttempts int) string {
	return usagePromptWithRetrySuffix(originalPrompt, retryAttempts, ToolOutputRetrySuffix)
}

func usagePromptWithRetrySuffix(originalPrompt string, retryAttempts int, suffix string) string {
	if retryAttempts <= 0 {
		return originalPrompt
	}
	parts := make([]string, 0, retryAttempts+1)
	parts = append(parts, originalPrompt)
	next := originalPrompt
	for i := 0; i < retryAttempts; i++ {
		next = strings.TrimRight(next, "\r\n\t ")
		if next == "" {
			next = suffix
		} else {
			next += "\n\n" + suffix
		}
		parts = append(parts, next)
	}
	return strings.Join(parts, "\n")
}
