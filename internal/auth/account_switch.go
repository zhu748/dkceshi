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
	a.accountSwitchFailure = ""
	if a.AccountPinned() {
		reason := "operation_pinned"
		if a.TargetAccount != "" {
			reason = "explicit_target"
		} else if a.FileAccountID != "" {
			reason = "client_file_owner"
		}
		config.Logger.Info("[account_switch] blocked", "account", a.AccountID, "reason", reason)
		a.accountSwitchFailure = reason
		return false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		a.accountSwitchFailure = "request_cancelled"
		return false
	}
	// Share the normal bounded queue rather than failing merely because the
	// replacement account is busy. The caller's earlier deadline always wins.
	if a.accountSwitches >= MaxAccountSwitches || a.failoverWait >= accountSwitchWaitTimeout {
		a.accountSwitchFailure = "account_switch_budget_exhausted"
		if a.failoverWait >= accountSwitchWaitTimeout {
			a.accountSwitchFailure = "account_switch_wait_exhausted"
		}
		config.Logger.Info("[account_switch] budget exhausted", "account", a.AccountID, "switches", a.accountSwitches)
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
			a.accountSwitchFailure = "account_switch_budget_exhausted"
			return false
		}
		acc, ok := r.Pool.AcquireWait(waitCtx, "", a.TriedAccounts, filter)
		if !ok {
			a.accountSwitchFailure = "no_eligible_account"
			if waitCtx.Err() != nil {
				a.accountSwitchFailure = "account_switch_wait_exhausted"
				if ctx.Err() != nil {
					a.accountSwitchFailure = "request_cancelled"
				}
			}
			config.Logger.Info("[account_switch] no eligible replacement", "account", a.AccountID, "tools_present", a.ToolsPresent, "tried_accounts", len(a.TriedAccounts), "error", waitCtx.Err())
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
		r.completeAccountAcquisition(a)
		return true
	}
}

// AccountSwitchFailureReason exposes the last scheduling decision without
// credentials so every completion surface can explain why failover stopped.
func (a *RequestAuth) AccountSwitchFailureReason() string {
	if a == nil || !a.UseConfigToken {
		return "unmanaged_account"
	}
	if a.resolver == nil {
		return "account_switch_unavailable"
	}
	return a.accountSwitchFailure
}
