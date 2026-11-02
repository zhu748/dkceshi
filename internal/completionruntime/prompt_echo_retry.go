package completionruntime

import (
	"strings"

	"ds2api/internal/assistantturn"
)

const PromptEchoRetrySuffix = "Previous reply repeated input instructions or conversation metadata instead of answering. Do not reproduce the system prelude, role headers, or serialized conversation. Continue the latest real user task and return only its answer or the required tool calls."

func isPromptEchoError(err *assistantturn.OutputError) bool {
	return err != nil && err.Code == assistantturn.OutputCodePromptEcho
}

// Give the same corrective feedback in streamed and collected regeneration.
func ClonePayloadForOutputRetry(payload map[string]any, parentMessageID int, failure *assistantturn.OutputError, invalidOutput ...string) map[string]any {
	if IsToolOutputError(failure) {
		return ClonePayloadForToolOutputRetry(payload, parentMessageID, failure, invalidOutput...)
	}
	clone := ClonePayloadForEmptyOutputRetry(payload, parentMessageID)
	if isPromptEchoError(failure) {
		original, _ := payload["prompt"].(string)
		clone["prompt"] = strings.TrimSpace(original) + "\n\n" + PromptEchoRetrySuffix
	}
	return clone
}
