package completionruntime

import (
	"net/http"

	"ds2api/internal/assistantturn"
	"ds2api/internal/auth"
)

// Recording an observed account failure does not depend on whether this
// request can retry, still has another account, or already wrote its terminal.
// Generic 503/network errors do not identify a throttled account.
func ObserveAccountOutputFailure(a *auth.RequestAuth, failure *assistantturn.OutputError) bool {
	if failure == nil || failure.Status != http.StatusTooManyRequests && !assistantturn.IsUnavailableEmptyOutput(failure) {
		return false
	}
	return a.NoteRateLimit()
}
