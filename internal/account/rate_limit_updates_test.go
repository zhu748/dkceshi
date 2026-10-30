package account

import (
	"testing"
	"time"

	"ds2api/internal/config"
)

func TestLoweringRateLimitAppliesToCurrentWindow(t *testing.T) {
	pool, store := newRateLimitedPoolForTest(t, true, 10, "account@example.com")
	t.Cleanup(pool.Close)
	pool.SetRateLimitWindow(time.Hour)
	for range 2 {
		acc, ok := pool.Acquire("", nil, nil)
		if !ok {
			t.Fatal("expected initial acquire")
		}
		pool.Release(acc.Identifier())
	}
	if err := store.Update(func(c *config.Config) error {
		c.Runtime.AccountRateLimit.MaxPerMinute = 2
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	pool.SyncRateLimits()
	if !pool.IsAccountRateLimited("account@example.com") {
		t.Fatal("lowered limit ignored existing requests in the active window")
	}
	if _, ok := pool.Acquire("", nil, nil); ok {
		t.Fatal("an extra request exceeded the new limit")
	}
}

func TestStaleRateLimitTimerCannotResetNewWindow(t *testing.T) {
	pool, _ := newRateLimitedPoolForTest(t, true, 2, "account@example.com")
	t.Cleanup(pool.Close)
	pool.SetRateLimitWindow(time.Hour)
	acc, ok := pool.Acquire("", nil, nil)
	if !ok {
		t.Fatal("initial acquire failed")
	}
	pool.Release(acc.Identifier())
	pool.mu.Lock()
	entry := pool.rateLimits[acc.Identifier()]
	oldWindow := entry.windowStart
	entry.windowStart = oldWindow.Add(time.Second)
	entry.count, entry.unavailable = 2, true
	pool.mu.Unlock()
	// A timer already waiting on the pool lock can run after a new window
	// starts, even when Stop was called on the old timer.
	pool.resetAccountRateLimit(acc.Identifier(), entry, oldWindow)
	if !pool.IsAccountRateLimited(acc.Identifier()) {
		t.Fatal("old timer cleared the current window's limit")
	}
	pool.resetAccountRateLimit(acc.Identifier(), &rateLimitEntry{}, entry.windowStart)
	if !pool.IsAccountRateLimited(acc.Identifier()) {
		t.Fatal("timer for a removed account entry cleared its replacement")
	}
	pool.resetAccountRateLimit(acc.Identifier(), entry, entry.windowStart)
	if pool.IsAccountRateLimited(acc.Identifier()) {
		t.Fatal("current window's timer did not reset its own counter")
	}
}
