package auth

import (
	"context"
	"testing"
	"time"

	"ds2api/internal/config"
)

func TestSchedulingFillsAccountConcurrencyOnlyInStickyMode(t *testing.T) {
	for _, sticky := range []bool{false, true} {
		r, pool := newSwitchWaitResolver(t)
		if err := r.Store.Update(func(c *config.Config) error {
			c.Runtime.AccountStickyEnabled = &sticky
			c.Runtime.AccountMaxInflight = 2
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		pool.Reset()
		var requests []*RequestAuth
		for i := 0; i < 4; i++ {
			requests = append(requests, determineManaged(t, r))
		}
		if (requests[0].AccountID == requests[1].AccountID) != sticky || requests[0].AccountID == requests[3].AccountID {
			t.Fatalf("sticky=%v accounts=%s,%s,%s,%s", sticky, requests[0].AccountID, requests[1].AccountID, requests[2].AccountID, requests[3].AccountID)
		}
		for _, a := range requests {
			r.Release(a)
		}
		if pool.Status()["in_use"] != 0 {
			t.Fatal("leaked concurrency slots")
		}
		pool.Close()
	}
}

func TestRateLimitBenchesTwoMinutesAndFailsOverInBothModes(t *testing.T) {
	for _, sticky := range []bool{false, true} {
		r, pool := newSwitchWaitResolver(t)
		if err := r.Store.Update(func(c *config.Config) error { c.Runtime.AccountStickyEnabled = &sticky; return nil }); err != nil {
			t.Fatal(err)
		}
		pool.Reset()
		a := determineManaged(t, r)
		old := a.AccountID
		before := time.Now()
		if !a.SwitchAccountOnRateLimit(context.Background()) || a.AccountID == old {
			t.Fatal("did not fail over")
		}
		acc, _ := r.Store.FindAccount(old)
		remaining := time.Unix(int64(acc.CooldownUntil), 0).Sub(before)
		if remaining < 119*time.Second || remaining > 121*time.Second || acc.Banned || acc.Disabled {
			t.Fatalf("wrong cooldown: %+v duration=%v", acc, remaining)
		}
		if a.SwitchAccountOnRateLimit(context.Background()) {
			t.Fatal("retried an already failed account")
		}
		r.Release(a)
		if pool.Status()["in_use"] != 0 {
			t.Fatalf("failed failover leaked slots: %+v", pool.Status())
		}
		pool.Close()
	}
}

func TestRateLimitDoesNotOverrideExplicitTarget(t *testing.T) {
	r, pool := newSwitchWaitResolver(t)
	t.Cleanup(pool.Close)
	a := determineManaged(t, r)
	t.Cleanup(func() { r.Release(a) })
	a.TargetAccount = a.AccountID
	old := a.AccountID
	if a.SwitchAccountOnRateLimit(context.Background()) || a.AccountID != old {
		t.Fatal("moved explicitly bound account")
	}
	acc, _ := r.Store.FindAccount(old)
	if !acc.IsCoolingDown() {
		t.Fatal("explicit binding prevented recording the rate limit")
	}
}
