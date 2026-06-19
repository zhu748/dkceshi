package client

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"ds2api/internal/auth"
)

func TestPowPrefetchCache_GetMiss(t *testing.T) {
	var calls int32
	cache := newPowPrefetchCache(func(ctx context.Context, a *auth.RequestAuth, targetPath string, maxAttempts int) (string, error) {
		atomic.AddInt32(&calls, 1)
		return "pow-fresh", nil
	})
	// 未 Put 过，应当 miss
	if pow, ok := cache.Get("acc1", "/api/v0/chat/completion"); ok {
		t.Fatalf("expected cache miss, got pow=%q", pow)
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatalf("fetcher should not be called on miss, got calls=%d", calls)
	}
}

func TestPowPrefetchCache_PutGetHit(t *testing.T) {
	cache := newPowPrefetchCache(func(ctx context.Context, a *auth.RequestAuth, targetPath string, maxAttempts int) (string, error) {
		t.Fatal("fetcher should not be called on cache hit")
		return "", nil
	})
	cache.Put("acc1", "/api/v0/chat/completion", "pow-cached", time.Now().Add(5*time.Minute))
	pow, ok := cache.Get("acc1", "/api/v0/chat/completion")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if pow != "pow-cached" {
		t.Fatalf("unexpected pow: %q", pow)
	}
	// 命中后应清空，再 Get 应 miss
	if _, ok := cache.Get("acc1", "/api/v0/chat/completion"); ok {
		t.Fatal("expected cache cleared after hit")
	}
}

func TestPowPrefetchCache_Expiry(t *testing.T) {
	cache := newPowPrefetchCache(func(ctx context.Context, a *auth.RequestAuth, targetPath string, maxAttempts int) (string, error) {
		return "pow-fresh", nil
	})
	// 已经过期（含 30s 安全余量）
	cache.Put("acc1", "/api/v0/chat/completion", "pow-old", time.Now().Add(-1*time.Second))
	if _, ok := cache.Get("acc1", "/api/v0/chat/completion"); ok {
		t.Fatal("expected cache miss for expired entry")
	}
}

func TestPowPrefetchCache_DifferentAccounts(t *testing.T) {
	cache := newPowPrefetchCache(func(ctx context.Context, a *auth.RequestAuth, targetPath string, maxAttempts int) (string, error) {
		return "pow-fresh", nil
	})
	cache.Put("acc1", "/api/v0/chat/completion", "pow-acc1", time.Now().Add(5*time.Minute))
	cache.Put("acc2", "/api/v0/chat/completion", "pow-acc2", time.Now().Add(5*time.Minute))
	pow1, ok := cache.Get("acc1", "/api/v0/chat/completion")
	if !ok || pow1 != "pow-acc1" {
		t.Fatalf("acc1 cache miss or wrong value: %q ok=%v", pow1, ok)
	}
	pow2, ok := cache.Get("acc2", "/api/v0/chat/completion")
	if !ok || pow2 != "pow-acc2" {
		t.Fatalf("acc2 cache miss or wrong value: %q ok=%v", pow2, ok)
	}
}

func TestPowPrefetchCache_DifferentTargetPaths(t *testing.T) {
	cache := newPowPrefetchCache(func(ctx context.Context, a *auth.RequestAuth, targetPath string, maxAttempts int) (string, error) {
		return "pow-fresh", nil
	})
	cache.Put("acc1", "/api/v0/chat/completion", "pow-completion", time.Now().Add(5*time.Minute))
	cache.Put("acc1", "/api/v0/file/upload_file", "pow-upload", time.Now().Add(5*time.Minute))
	pow, ok := cache.Get("acc1", "/api/v0/chat/completion")
	if !ok || pow != "pow-completion" {
		t.Fatalf("completion pow mismatch: %q ok=%v", pow, ok)
	}
	pow, ok = cache.Get("acc1", "/api/v0/file/upload_file")
	if !ok || pow != "pow-upload" {
		t.Fatalf("upload pow mismatch: %q ok=%v", pow, ok)
	}
}

func TestPowPrefetchCache_SchedulePrefetchWritesCache(t *testing.T) {
	var calls int32
	cache := newPowPrefetchCache(func(ctx context.Context, a *auth.RequestAuth, targetPath string, maxAttempts int) (string, error) {
		atomic.AddInt32(&calls, 1)
		return "pow-prefetched", nil
	})
	a := &auth.RequestAuth{AccountID: "acc1"}
	cache.SchedulePrefetch(context.Background(), a, "/api/v0/chat/completion", 3)

	// 等待异步预取完成
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if pow, ok := cache.Get("acc1", "/api/v0/chat/completion"); ok && pow == "pow-prefetched" {
			return // success
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("prefetch did not populate cache within 2s, calls=%d", atomic.LoadInt32(&calls))
}

func TestPowPrefetchCache_SchedulePrefetchDeduplicates(t *testing.T) {
	var calls int32
	cache := newPowPrefetchCache(func(ctx context.Context, a *auth.RequestAuth, targetPath string, maxAttempts int) (string, error) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(50 * time.Millisecond) // 模拟慢 PoW
		return "pow-prefetched", nil
	})
	a := &auth.RequestAuth{AccountID: "acc1"}
	// 连续调度 5 次预取，应只触发 1 次 fetcher（首次）
	for i := 0; i < 5; i++ {
		cache.SchedulePrefetch(context.Background(), a, "/api/v0/chat/completion", 3)
	}
	time.Sleep(200 * time.Millisecond) // 等待预取完成
	if got := atomic.LoadInt32(&calls); got > 1 {
		t.Fatalf("expected at most 1 fetcher call during concurrent prefetch, got %d", got)
	}
}

func TestPowPrefetchCache_SchedulePrefetchOnError(t *testing.T) {
	var calls int32
	cache := newPowPrefetchCache(func(ctx context.Context, a *auth.RequestAuth, targetPath string, maxAttempts int) (string, error) {
		atomic.AddInt32(&calls, 1)
		return "", errors.New("network error")
	})
	a := &auth.RequestAuth{AccountID: "acc1"}
	cache.SchedulePrefetch(context.Background(), a, "/api/v0/chat/completion", 3)
	time.Sleep(200 * time.Millisecond)
	// 预取失败：缓存应无条目
	if cache.Size() != 0 {
		t.Fatalf("expected empty cache after failed prefetch, got size=%d", cache.Size())
	}
	// 之后再调用 SchedulePrefetch 应能再次触发 fetcher（因为失败的 fetching 标记已清除）
	cache.SchedulePrefetch(context.Background(), a, "/api/v0/chat/completion", 3)
	time.Sleep(200 * time.Millisecond)
	if got := atomic.LoadInt32(&calls); got < 2 {
		t.Fatalf("expected fetcher to be retried after failure, got calls=%d", got)
	}
}

func TestPowPrefetchCache_Clear(t *testing.T) {
	cache := newPowPrefetchCache(func(ctx context.Context, a *auth.RequestAuth, targetPath string, maxAttempts int) (string, error) {
		return "pow-fresh", nil
	})
	cache.Put("acc1", "/api/v0/chat/completion", "pow-1", time.Now().Add(5*time.Minute))
	cache.Put("acc2", "/api/v0/chat/completion", "pow-2", time.Now().Add(5*time.Minute))
	if cache.Size() != 2 {
		t.Fatalf("expected 2 entries, got %d", cache.Size())
	}
	cache.Clear()
	if cache.Size() != 0 {
		t.Fatalf("expected 0 entries after Clear, got %d", cache.Size())
	}
}

func TestPowPrefetchCache_ClearForAccount(t *testing.T) {
	cache := newPowPrefetchCache(func(ctx context.Context, a *auth.RequestAuth, targetPath string, maxAttempts int) (string, error) {
		return "pow-fresh", nil
	})
	cache.Put("acc1", "/api/v0/chat/completion", "pow-1", time.Now().Add(5*time.Minute))
	cache.Put("acc1", "/api/v0/file/upload_file", "pow-1u", time.Now().Add(5*time.Minute))
	cache.Put("acc2", "/api/v0/chat/completion", "pow-2", time.Now().Add(5*time.Minute))
	cache.ClearForAccount("acc1")
	if cache.Size() != 1 {
		t.Fatalf("expected 1 entry after ClearForAccount(acc1), got %d", cache.Size())
	}
	if _, ok := cache.Get("acc2", "/api/v0/chat/completion"); !ok {
		t.Fatal("acc2 cache should still exist after clearing acc1")
	}
}
