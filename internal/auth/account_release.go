package auth

import "context"

// releaseAccountSlot returns one account's slot to the pool, at most once per
// account per request.
//
// SwitchAccount hands the current account back before it looks for a replacement
// — that is what lets a request fail over while the pool is at its global
// in-flight limit — so a failed switch leaves a.AccountID naming an account whose
// slot is already gone. Without this bookkeeping the request-end Release would
// decrement it again, and the pool would then count one holder fewer than it has:
// with two requests sharing an account and one of them failing over, the account
// would drop to zero holders while the other request is still streaming, and the
// next request would be admitted on top of it.
func (r *Resolver) releaseAccountSlot(a *RequestAuth, accountID string) {
	if r == nil || a == nil || accountID == "" {
		return
	}
	if a.releasedAccounts[accountID] {
		return
	}
	if a.releasedAccounts == nil {
		a.releasedAccounts = map[string]bool{}
	}
	a.releasedAccounts[accountID] = true
	r.Pool.Release(accountID)
}

func (a *RequestAuth) SwitchAccount(ctx context.Context) bool {
	if a == nil || a.resolver == nil {
		return false
	}
	return a.resolver.SwitchAccount(ctx, a)
}

// Release 结束一次托管请求：结算连续鉴权失败计数并归还账号名额。名额的归还走
// releaseAccountSlot，因此 SwitchAccount 已经归还过的账号不会被减第二次。
func (r *Resolver) Release(a *RequestAuth) {
	a.FinalizeAccountResources()
	if a == nil || !a.UseConfigToken || a.AccountID == "" {
		return
	}
	r.settleAuthFailures(a)
	r.releaseAccountSlot(a, a.AccountID)
}
