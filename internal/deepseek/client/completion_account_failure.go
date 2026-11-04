package client

import (
	"context"
	"net/http"

	"ds2api/internal/auth"
)

func (c *Client) inspectCompletionFailure(ctx context.Context, a *auth.RequestAuth, resp *http.Response, source string) error {
	body, parsed, err := inspectCompletionJSONContext(ctx, resp.Body)
	if body != nil {
		resp.Body = body
	}
	if err != nil {
		return err
	}
	code, bizCode, msg, bizMsg := extractResponseStatus(parsed)
	if limited := fileRateLimit("completion", resp.StatusCode, code, bizCode, msg, bizMsg); limited != nil {
		a.NoteRateLimit()
		return limited
	}
	if kind := c.observeAccountFailure(a, parsed, source); kind != FailureUnknown {
		return &RequestFailure{Op: "completion", Kind: kind, Message: failureMessage(msg, bizMsg, "account unavailable")}
	}
	// A JSON auth rejection can arrive with HTTP 200. Keep the session owner
	// fixed here; the outer runtime owns refresh and whole-request replay.
	if parsed != nil && (resp.StatusCode == http.StatusUnauthorized || shouldAttemptRefresh(http.StatusOK, code, bizCode, msg, bizMsg)) {
		return &RequestFailure{Op: "completion", Kind: authFailureKind(a != nil && a.UseConfigToken), Message: failureMessage(msg, bizMsg, "token rejected")}
	}
	return nil
}
