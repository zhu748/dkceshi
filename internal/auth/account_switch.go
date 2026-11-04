package auth

import (
	"context"
	"errors"
	"time"

	"ds2api/internal/account"
	"ds2api/internal/config"
)

// Bound failover waits even when the incoming request has no deadline. Queue
// capacity, account filters and exhausted-account exclusions still apply.
const accountSwitchWaitTimeout = 30 * time.Second

// One budget spans upload, context, session, PoW and completion recovery. Token
// refreshes on the current account do not consume replacement-account slots.
const MaxAccountSwitches = 3

func (a *RequestAuth) accountSwitchContext(ctx context.Context) (context.Context, context.CancelFunc) {
	remaining := accountSwitchWaitTimeout - a.failoverWait
	started := time.Now()
	bounded, cancel := context.WithTimeout(ctx, max(remaining, 0))
	return bounded, func() {
		a.failoverWait += time.Since(started)
		cancel()
	}
}

// SwitchAccount 把当前请求切到下一个可调度账号，并确保新账号持有可用 Token。
// 逐个尝试，登录被上游明确拒绝（ErrLoginRejected）的账号同样计入连续鉴权失败，
// 达到阈值后会被移出号池；全部账号都不可用时返回 false。
func (r *Resolver) SwitchAccount(ctx context.Context, a *RequestAuth) bool {
	if a == nil || !a.UseConfigToken {
		return false
	}
	if a.AccountPinned() {
		return false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return false
	}
	// Share the normal bounded queue rather than failing merely because the
	// replacement account is busy. The caller's earlier deadline always wins.
	if a.accountSwitches >= MaxAccountSwitches || a.failoverWait >= accountSwitchWaitTimeout {
		return false
	}
	waitCtx, cancel := a.accountSwitchContext(ctx)
	defer cancel()
	if a.TriedAccounts == nil {
		a.TriedAccounts = map[string]bool{}
	}
	if a.AccountID != "" {
		a.CleanupAccountResources()
		a.TriedAccounts[a.AccountID] = true
		r.releaseAccountSlot(a, a.AccountID)
	}
	filter := account.AccountFilter(func(acc config.Account) bool {
		return acc.MatchesPoolType(a.ToolsPresent)
	})
	for {
		if a.accountSwitches >= MaxAccountSwitches {
			return false
		}
		acc, ok := r.Pool.AcquireWait(waitCtx, "", a.TriedAccounts, filter)
		if !ok {
			// No replacement is available. a.AccountID still names the account whose
			// slot this call just returned, and releaseAccountSlot remembers that, so
			// the request-end Release will not return it a second time.
			return false
		}
		a.Account = acc
		a.AccountID = acc.Identifier()
		a.accountSwitches++
		if err := r.ensureManagedToken(waitCtx, a); err != nil {
			a.TriedAccounts[a.AccountID] = true
			r.releaseAccountSlot(a, a.AccountID)
			if errors.Is(err, ErrLoginRejected) {
				r.MarkAuthFailure(a, authFailedReasonFromLoginError(err))
			}
			continue
		}
		a.markAccountUsed()
		if r.OnAccountUsed != nil {
			r.OnAccountUsed(a.AccountID)
		}
		return true
	}
}
