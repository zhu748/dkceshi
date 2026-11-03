package assistantturn

import (
	"net/http"
	"strings"
)

const UpstreamUnavailableEmptyMessage = "Upstream service is unavailable and returned no output."

// This known empty-response outcome uses the account cooldown policy. Generic
// network failures and unrelated 503 errors retain their normal retry policy.
func IsUnavailableEmptyOutput(err *OutputError) bool {
	return err != nil && err.Status == http.StatusServiceUnavailable &&
		err.Code == "upstream_unavailable" && strings.TrimSpace(err.Message) == UpstreamUnavailableEmptyMessage
}
