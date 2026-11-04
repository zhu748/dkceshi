package completionruntime

import (
	"context"
	"net/http"

	"ds2api/internal/assistantturn"
	"ds2api/internal/auth"
	"ds2api/internal/config"
	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
)

// Collect transport state only. The retry loop builds and validates one turn
// per attempt, after merging its reasoning; incomplete reads never run repair.
func collectAttempt(ctx context.Context, resp *http.Response, a *auth.RequestAuth, req promptcompat.StandardRequest) (sse.CollectResult, *assistantturn.OutputError) {
	if outErr := completionResponseError(resp, a); outErr != nil {
		return sse.CollectResult{}, outErr
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			config.Logger.Warn("[completion_runtime] response body close failed", "surface", req.Surface, "error", err)
		}
	}()
	return sse.CollectStreamWithOptions(resp, req.Thinking, false, sse.CollectOptions{Context: ctx}, req.FinalPrompt), nil
}
