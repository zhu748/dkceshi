package auth

import (
	"context"
	"ds2api/internal/config"
	"fmt"
	"testing"
	"time"
)

func TestAccountFailoverBudgetSpansAllSwitches(t *testing.T) {
	r, p := newSwitchWaitResolver(t)
	defer p.Close()
	if err := r.Store.Update(func(c *config.Config) error {
		for i := 0; i < 8; i++ {
			c.Accounts = append(c.Accounts, config.Account{Email: fmt.Sprintf("extra%d@example.com", i), Token: "token"})
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	p.Reset()
	a := determineManaged(t, r)
	var cleaned []string
	for i := 0; i < MaxAccountSwitches; i++ {
		old := a.AccountID
		a.SetAccountCleanup(func() { cleaned = append(cleaned, a.AccountID) })
		if !a.SwitchAccount(context.Background()) {
			t.Fatalf("switch %d failed", i)
		}
		if cleaned[len(cleaned)-1] != old {
			t.Fatal("cleanup ran after owner changed")
		}
	}
	if a.SwitchAccount(context.Background()) {
		t.Fatal("cross-layer failover exceeded its budget")
	}
	if len(cleaned) != MaxAccountSwitches {
		t.Fatalf("cleanup calls=%v", cleaned)
	}
	r.Release(a)
	r.Release(a)
	if p.Status()["in_use"] != 0 {
		t.Fatal("budget exhaustion leaked account slots")
	}
}

func TestAccountFailoverSharesRemainingWaitAndHonorsEarlierDeadline(t *testing.T) {
	a := &RequestAuth{failoverWait: accountSwitchWaitTimeout - 20*time.Millisecond}
	ctx, cancel := a.accountSwitchContext(context.Background())
	deadline, _ := ctx.Deadline()
	if remaining := time.Until(deadline); remaining > 20*time.Millisecond {
		t.Fatalf("remaining=%s", remaining)
	}
	<-ctx.Done()
	cancel()
	if a.failoverWait < accountSwitchWaitTimeout {
		t.Fatal("elapsed wait was not charged")
	}
	earlier, done := context.WithCancel(context.Background())
	done()
	ctx, cancel = a.accountSwitchContext(earlier)
	defer cancel()
	if ctx.Err() == nil {
		t.Fatal("ignored canceled parent")
	}
}
