package client

import (
	"ds2api/internal/auth"
)

// observeAccountFailure updates shared account state before a request switches
// accounts. A captcha is surfaced to callers; this does not solve the challenge.
func (c *Client) observeAccountFailure(a *auth.RequestAuth, response map[string]any, op string) FailureKind {
	if a == nil {
		return FailureUnknown
	}
	if isMutedJSONResponse(response) {
		c.persistMutedUntil(a.AccountID, extractMuteUntil(response), op)
		return FailureMuted
	}
	if DetectCaptchaChallenge(response) != nil {
		c.coolDownAfterCaptcha(a, op)
		return FailureCaptchaRequired
	}
	return FailureUnknown
}
