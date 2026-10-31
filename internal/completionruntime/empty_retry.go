package completionruntime

import (
	"strings"

	"ds2api/internal/assistantturn"
)

const EmptyOutputRetrySuffix = "Previous reply had no visible output. Please regenerate the visible final answer or tool call now."

const ToolOutputRetrySuffix = "Previous reply contained an invalid tool call. Regenerate the answer using only the declared tools, valid complete JSON arguments matching their schemas, and the requested tool_choice and parallel_tool_calls policy."

func ClonePayloadForToolOutputRetry(payload map[string]any, parentMessageID int, failure *assistantturn.OutputError) map[string]any {
	clone := ClonePayloadForEmptyOutputRetry(payload, parentMessageID)
	original, _ := payload["prompt"].(string)
	clone["prompt"] = strings.TrimRight(original, "\r\n\t ") + "\n\n" + toolOutputRetryInstruction(failure)
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

// UsagePromptWithRetryPayload counts the exact follow-up prompt sent upstream,
// once per attempt. Rebuilding earlier attempts with the newest validation
// error would miscount them and recursively duplicate context on later retries.
func UsagePromptWithRetryPayload(previous string, payload map[string]any) string {
	prompt, _ := payload["prompt"].(string)
	return previous + "\n" + prompt
}
