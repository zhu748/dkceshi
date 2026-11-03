package client

import (
	"net/http"

	"ds2api/internal/auth"
)

func (c *Client) inspectCompletionFailure(a *auth.RequestAuth, resp *http.Response, source string) error {
	body, parsed, err := inspectCompletionJSON(resp.Body)
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
	return nil
}
