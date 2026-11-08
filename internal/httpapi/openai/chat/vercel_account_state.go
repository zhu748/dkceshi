package chat

import (
	"ds2api/internal/assistantturn"
	"ds2api/internal/auth"
	"ds2api/internal/completionruntime"
	"ds2api/internal/util"
)

// A terminal Node failure still updates account health when no switch can run.
// Bind the observation to its actual completion owner: a failed switch may have
// moved Auth onto an account that has never generated this completion.
func (h *Handler) observeReleasedAccountFailure(a *auth.RequestAuth, req map[string]any) {
	if a == nil || a.AccountID == "" || asString(req["failure_account_id"]) != a.AccountID {
		return
	}
	failure := &assistantturn.OutputError{Status: util.IntFrom(req["error_status"]), Code: asString(req["error_code"]), Message: asString(req["error"])}
	completionruntime.ObserveAccountOutputFailure(a, failure)
	options, _ := req["account_failure"].(map[string]any)
	if !a.UseConfigToken {
		return
	}
	if failure.Code == "account_banned" && options["banned"] == true {
		if notifier, ok := h.Auth.(interface {
			SetAccountBanned(*auth.RequestAuth, string)
		}); ok {
			notifier.SetAccountBanned(a, asString(options["banned_reason"]))
		}
	}
	if until := float64(util.IntFrom(options["mute_until"])); failure.Code == "account_muted" && until > 0 {
		if notifier, ok := h.Auth.(interface {
			SetAccountMutedUntil(*auth.RequestAuth, float64)
		}); ok {
			notifier.SetAccountMutedUntil(a, until)
		}
	}
}
