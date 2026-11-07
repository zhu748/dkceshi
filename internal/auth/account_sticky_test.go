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

func TestRateLimitBenchesOneMinuteAndFailsOverInBothModes(t *testing.T) {
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
		after := time.Now()
		acc, _ := r.Store.FindAccount(old)
		expiry := time.Unix(int64(acc.CooldownUntil), 0)
		if expiry.Before(before.Add(59*time.Second)) || expiry.After(after.Add(time.Minute)) || acc.Banned || acc.Disabled {
			t.Fatalf("wrong cooldown: %+v duration=%v", acc, expiry.Sub(before))
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

func TestRateLimitRetainsExistingLongerCooldown(t *testing.T) {
	r, pool := newSwitchWaitResolver(t)
	t.Cleanup(pool.Close)
	a := determineManaged(t, r)
	t.Cleanup(func() { r.Release(a) })
	until := float64(time.Now().Add(5 * time.Minute).Unix())
	if err := r.Store.Update(func(c *config.Config) error {
		for i := range c.Accounts {
			if c.Accounts[i].Identifier() == a.AccountID {
				c.Accounts[i].CooldownUntil = until
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !a.NoteRateLimit() {
		t.Fatal("managed account did not record rate limit")
	}
	acc, _ := r.Store.FindAccount(a.AccountID)
	if acc.CooldownUntil != until || a.Account.CooldownUntil != until {
		t.Fatal("new one-minute policy shortened an existing restriction")
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
