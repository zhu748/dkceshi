package account

import (
	"context"
	"testing"
	"time"
)

func TestLazyRateExpiryWakesQueuedRequest(t *testing.T) {
	const id = "one@example.com"
	pool, _ := newRateLimitedPoolForTest(t, true, 1, id)
	defer pool.Close()
	pool.SetRateLimitWindow(time.Hour)
	if !acquireOnce(t, pool) {
		t.Fatal("first acquire")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result := make(chan bool, 1)
	go func() {
		acc, ok := pool.AcquireWait(ctx, id, nil, nil)
		if ok {
			pool.Release(acc.Identifier())
		}
		result <- ok
	}()
	waitForWaitingCount(t, pool, 1)
	// Advance only the logical window. The real timer remains pending, as
	// when a status request acquires the lock just before the timer callback.
	pool.mu.Lock()
	pool.rateLimits[id].windowStart = time.Now().Add(-2 * time.Hour)
	pool.mu.Unlock()
	if pool.IsAccountRateLimited(id) {
		t.Fatal("expired rate limit still active")
	}
	if !<-result {
		t.Fatal("lazy expiry stopped the reset timer without waking its waiter")
	}
}

func TestLazyRateExpiryDuringNotification(t *testing.T) {
	const id = "one@example.com"
	pool, _ := newRateLimitedPoolForTest(t, true, 1, id)
	defer pool.Close()
	pool.SetRateLimitWindow(time.Hour)
	if !acquireOnce(t, pool) {
		t.Fatal("first acquire")
	}
	first := &poolWaiter{ready: make(chan struct{}), target: id}
	second := &poolWaiter{ready: make(chan struct{})}
	pool.mu.Lock()
	defer pool.mu.Unlock()
	pool.waiters = []*poolWaiter{first, second}
	pool.rateLimits[id].windowStart = time.Now().Add(-2 * time.Hour)
	pool.notifyWaiterLocked()
	for _, waiter := range []*poolWaiter{first, second} {
		select {
		case <-waiter.ready:
		default:
			t.Fatal("expiry notification stranded a waiter")
		}
	}
}
