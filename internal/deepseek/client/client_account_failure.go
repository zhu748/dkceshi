package client

import (
	"strings"

	"ds2api/internal/auth"
)

// observeAccountFailure updates shared account state before a request switches
// accounts. A captcha is surfaced to callers; this does not solve the challenge.
func (c *Client) observeAccountFailure(a *auth.RequestAuth, response map[string]any, op string) FailureKind {
	if a == nil {
		return FailureUnknown
	}
	_, bizCode, msg, bizMsg := extractResponseStatus(response)
	if isUserBannedResponse(bizCode, bizMsg) || strings.Contains(strings.ToUpper(msg), "USER_IS_BANNED") {
		c.persistBannedAccount(a.AccountID, "USER_IS_BANNED", op)
		a.Account.Banned, a.Account.Disabled = true, true
		return FailureBanned
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
