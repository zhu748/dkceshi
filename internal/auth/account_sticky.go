package auth

import (
	"context"
	"time"

	"ds2api/internal/account"
	"ds2api/internal/config"
)

// ManagedPool reports whether shared failover owns this request's account.
func (a *RequestAuth) ManagedPool() bool {
	return a != nil && a.UseConfigToken && a.resolver != nil && a.resolver.Store != nil && a.resolver.Pool != nil
}

// NoteRateLimit only benches an account; callers that own replayable inputs
// decide when it is safe to rebuild a request on a different account.
func (a *RequestAuth) NoteRateLimit() bool {
	if !a.ManagedPool() || a.AccountID == "" {
		return false
	}
	r := a.resolver
	until := float64(time.Now().Add(time.Minute).Unix())
	if current, ok := r.Store.FindAccount(a.AccountID); ok && current.CooldownUntil >= until-1 {
		a.Account.CooldownUntil = current.CooldownUntil
		return true
	}
	busy := r.inflightAccounts()
	err := r.Store.Update(func(cfg *config.Config) error {
		for i := range cfg.Accounts {
			if cfg.Accounts[i].Identifier() == a.AccountID {
				cfg.Accounts[i].CooldownUntil = max(cfg.Accounts[i].CooldownUntil, until)
			}
		}
		if cfg.ElasticPool.Enabled {
			account.ReconcileElasticPoolWithBusy(cfg, busy)
		}
		return nil
	})
	if err != nil {
		config.Logger.Warn("[account_sticky] failed to persist rate-limit cooldown", "error", err, "account", a.AccountID)
	}
	a.Account.CooldownUntil = max(a.Account.CooldownUntil, until)
	if r.Pool != nil {
		r.Pool.Reset()
	}
	if r.OnAccountPoolChanged != nil {
		r.OnAccountPoolChanged()
	}
	return true
}

func (a *RequestAuth) SwitchAccountOnRateLimit(ctx context.Context) bool {
	return a.NoteRateLimit() && a.SwitchAccount(ctx)
}
