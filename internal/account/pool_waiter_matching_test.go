package account

import (
	"context"
	"testing"
	"time"
)

func TestReleaseWakesMatchingWaiterPastBusyPool(t *testing.T) {
	pool := newMixedPoolForTest(t)
	for _, id := range []string{"acc1@example.com", "acc2@example.com", "acc3@example.com"} {
		if _, ok := pool.Acquire(id, nil, nil); !ok {
			t.Fatalf("occupy %s", id)
		}
		defer pool.Release(id)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	first := make(chan bool, 1)
	go func() { _, ok := pool.AcquireWait(ctx, "acc3@example.com", nil, toolsPresentFilter(true)); first <- ok }()
	waitForWaitingCount(t, pool, 1)
	second := make(chan bool, 1)
	go func() {
		_, ok := pool.AcquireWait(ctx, "acc2@example.com", nil, toolsPresentFilter(false))
		second <- ok
	}()
	waitForWaitingCount(t, pool, 2)
	pool.Release("acc2@example.com")
	select {
	case ok := <-second:
		if !ok {
			t.Fatal("idle no-tools account was stranded behind a tools-only waiter")
		}
	case <-ctx.Done():
		t.Fatal("matching waiter was not woken")
	}
	select {
	case <-first:
		t.Fatal("busy tools-only waiter should still be waiting")
	default:
	}
	cancel()
	if <-first {
		t.Fatal("cancelled waiter acquired an account")
	}
	waitForWaitingCount(t, pool, 0)
}

func TestIncreasingCapacityWakesAllUsableWaiters(t *testing.T) {
	pool := newSingleAccountPoolForTest(t, "1")
	pool.ApplyRuntimeLimits(1, 3, 1)
	acc, ok := pool.Acquire("", nil, nil)
	if !ok {
		t.Fatal("occupy account")
	}
	defer pool.Release(acc.Identifier())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	results := make(chan bool, 2)
	for range 2 {
		go func() { _, ok := pool.AcquireWait(ctx, "", nil, nil); results <- ok }()
	}
	waitForWaitingCount(t, pool, 2)
	pool.ApplyRuntimeLimits(3, 3, 3)
	for range 2 {
		select {
		case ok := <-results:
			if !ok {
				t.Fatal("waiter did not acquire newly available capacity")
			}
		case <-ctx.Done():
			t.Fatal("capacity increase stranded a waiter")
		}
		defer pool.Release(acc.Identifier())
	}
}
