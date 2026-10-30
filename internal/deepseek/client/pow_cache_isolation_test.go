package client

import (
	"context"
	"fmt"
	"testing"
	"time"

	"ds2api/internal/auth"
)

func waitForPowEntries(t *testing.T, cache *powPrefetchCache, want int) {
	t.Helper()
	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		cache.mu.Lock()
		count := 0
		for _, entry := range cache.entries {
			if entry.pow != "" && !entry.fetching {
				count++
			}
		}
		cache.mu.Unlock()
		if count == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("expected %d independent proof entries, cache size=%d", want, cache.Size())
}

func TestDirectTokensDoNotSharePrefetchedPow(t *testing.T) {
	cache := newPowPrefetchCache(func(_ context.Context, a *auth.RequestAuth, _ string, _ int) (powSolution, error) {
		return powSolution{header: "proof-for-" + a.DeepSeekToken, expiresAt: time.Now().Add(time.Hour)}, nil
	})
	a := &auth.RequestAuth{DeepSeekToken: "token-a"}
	b := &auth.RequestAuth{DeepSeekToken: "token-b"}
	const target = "/api/v0/chat/completion"
	cache.SchedulePrefetch(context.Background(), a, target, 1)
	waitForPowEntries(t, cache, 1)
	cache.SchedulePrefetch(context.Background(), b, target, 1)
	waitForPowEntries(t, cache, 2)
	client := &Client{powCache: cache}
	for _, identity := range []*auth.RequestAuth{b, a} {
		proof, err := client.GetPowForTarget(context.Background(), identity, target, 1)
		if err != nil || proof != "proof-for-"+identity.DeepSeekToken {
			t.Fatalf("wrong identity's proof: token=%s proof=%s error=%v", identity.DeepSeekToken, proof, err)
		}
	}
}

func TestPowCacheBoundsDistinctIdentitiesAndReclaimsExpiredEntries(t *testing.T) {
	cache := newPowPrefetchCache(nil)
	for i := range 300 {
		cache.Put(fmt.Sprint(i), "/target", "proof", time.Now().Add(time.Hour))
	}
	if cache.Size() > 256 {
		t.Fatalf("proof cache grows without a limit: %d", cache.Size())
	}
	cache.mu.Lock()
	for _, entry := range cache.entries {
		entry.expireAt = time.Now().Add(-time.Hour)
	}
	cache.mu.Unlock()
	cache.Put("fresh", "/target", "fresh-proof", time.Now().Add(time.Hour))
	if cache.Size() != 1 {
		t.Fatalf("expired identities remain after a new write: %d", cache.Size())
	}
}

func TestPowCacheDoesNotEvictActivePrefetchesWhenFull(t *testing.T) {
	release := make(chan struct{})
	cache := newPowPrefetchCache(func(context.Context, *auth.RequestAuth, string, int) (powSolution, error) {
		<-release
		return powSolution{header: "proof", expiresAt: time.Now().Add(time.Hour)}, nil
	})
	t.Cleanup(func() { cache.Clear(); close(release) })
	for i := range 300 {
		cache.SchedulePrefetch(context.Background(), &auth.RequestAuth{AccountID: fmt.Sprint(i)}, "/target", 1)
	}
	if got := cache.Size(); got != 256 {
		t.Fatalf("in-flight prefetch count exceeded its budget: %d", got)
	}
	cache.Put("new", "/target", "synchronous-proof", time.Now().Add(time.Hour))
	cache.mu.Lock()
	defer cache.mu.Unlock()
	for _, entry := range cache.entries {
		if !entry.fetching {
			t.Fatal("active prefetch was evicted for a synchronous cache write")
		}
	}
}
