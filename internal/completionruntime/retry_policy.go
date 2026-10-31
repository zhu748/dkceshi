package completionruntime

import (
	"net/http"

	"ds2api/internal/assistantturn"
)

// IsToolOutputError identifies a model-generated tool call that failed output
// validation. Invalid incoming requests are rejected before this stage.
func IsToolOutputError(err *assistantturn.OutputError) bool {
	return err != nil && (err.Code == "invalid_tool_arguments" || err.Code == "tool_choice_violation")
}

// IsRetryableOutputError is shared by protocol adapters and the retry runtime.
// Content filtering, input errors and caller cancellations are never retried.
func IsRetryableOutputError(err *assistantturn.OutputError) bool {
	if err == nil {
		return false
	}
	switch err.Code {
	case assistantturn.OutputCodePromptEcho, "upstream_empty_output", "upstream_unavailable", "account_muted", OutputCodeAccountUnauthorized, "captcha_required":
		return true
	}
	return IsToolOutputError(err) || isRetryableUpstreamError(err) || err.Code == "error" &&
		(err.Status == http.StatusTooManyRequests || IsTransientUpstreamStatus(err.Status))
}

// CanDeferOutputError only allows regeneration before text or a tool call has
// been delivered. Reasoning and whitespace alone are not a completed answer;
// adapters must not mark them as outputDelivered.
func CanDeferOutputError(err *assistantturn.OutputError, retryEnabled, outputDelivered bool) bool {
	return retryEnabled && !outputDelivered && IsRetryableOutputError(err)
}
