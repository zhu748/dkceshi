package client

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestUploadCacheConcurrentRequestsAndMetadataIsolation(t *testing.T) {
	cache := newUploadReuseCache(time.Minute, 4)
	var uploads atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan *UploadFileResult, 32)
	for range 32 {
		wg.Go(func() {
			<-start
			result, err := cache.do(context.Background(), "same", "owner", func(cached *UploadFileResult) (*UploadFileResult, bool, error) {
				if cached == nil {
					uploads.Add(1)
					cached = &UploadFileResult{ID: "one", Status: "PENDING", Raw: map[string]any{"signed_path": "private"}}
				}
				return cached, true, nil
			})
			if err != nil {
				t.Error(err)
			}
			results <- result
		})
	}
	close(start)
	wg.Wait()
	close(results)
	if uploads.Load() != 1 {
		t.Fatalf("concurrent requests uploaded %d times", uploads.Load())
	}
	for result := range results {
		if result == nil || result.ID != "one" {
			t.Fatalf("result: %#v", result)
		}
		result.Status = "caller-mutated"
	}
	entry := cache.entries["same"].file
	if entry.Status != "PENDING" || entry.Raw != nil || entry.RawHeaders != nil {
		t.Fatalf("cache retained mutable/private response: %#v", entry)
	}
}

func TestUploadCacheTTLBoundAndNonSlidingExpiry(t *testing.T) {
	cache := newUploadReuseCache(time.Minute, 2)
	now := time.Unix(100, 0)
	cache.now = func() time.Time { return now }
	uploads := 0
	load := func(cached *UploadFileResult) (*UploadFileResult, bool, error) {
		if cached == nil {
			uploads++
			cached = &UploadFileResult{ID: fmt.Sprintf("file-%d", uploads), Status: "SUCCESS"}
		}
		return cached, true, nil
	}
	call := func(key string) {
		t.Helper()
		if _, err := cache.do(context.Background(), key, "owner", load); err != nil {
			t.Fatal(err)
		}
	}
	call("a")
	now = now.Add(30 * time.Second)
	call("a")
	now = now.Add(30 * time.Second)
	call("a")
	if uploads != 2 {
		t.Fatalf("TTL was extended by a cache hit: uploads=%d", uploads)
	}
	now = now.Add(time.Second)
	call("b")
	now = now.Add(time.Second)
	call("c")
	if len(cache.entries) != 2 || cache.entries["a"].file != nil {
		t.Fatalf("bounded eviction failed: %#v", cache.entries)
	}
}

func TestUploadCacheCancellationAndFailureRecovery(t *testing.T) {
	cache := newUploadReuseCache(time.Minute, 2)
	leaderCtx, cancel := context.WithCancel(context.Background())
	started, leaderDone := make(chan struct{}), make(chan error, 1)
	go func() {
		_, err := cache.do(leaderCtx, "same", "owner", func(*UploadFileResult) (*UploadFileResult, bool, error) {
			close(started)
			<-leaderCtx.Done()
			return nil, false, leaderCtx.Err()
		})
		leaderDone <- err
	}()
	<-started
	waiterCtx, stop := context.WithTimeout(context.Background(), 3*time.Second)
	defer stop()
	waiterDone := make(chan error, 1)
	go func() {
		result, err := cache.do(waiterCtx, "same", "owner", func(*UploadFileResult) (*UploadFileResult, bool, error) {
			return &UploadFileResult{ID: "recovered", Status: "SUCCESS"}, true, nil
		})
		if err == nil && (result == nil || result.ID != "recovered") {
			err = errors.New("waiter received canceled leader result")
		}
		waiterDone <- err
	}()
	cancel()
	if err := <-leaderDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("leader error: %v", err)
	}
	if err := <-waiterDone; err != nil {
		t.Fatal(err)
	}
	canceled, stopCanceled := context.WithCancel(context.Background())
	stopCanceled()
	if _, err := cache.do(canceled, "same", "owner", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled cache hit: %v", err)
	}
}

func TestUploadCacheInvalidationPreventsInflightResurrection(t *testing.T) {
	cache := newUploadReuseCache(time.Minute, 2)
	_, err := cache.do(context.Background(), "key", "owner", func(*UploadFileResult) (*UploadFileResult, bool, error) {
		cache.invalidateOwner("owner")
		return &UploadFileResult{ID: "old-file", Status: "SUCCESS"}, true, nil
	})
	if err != nil || len(cache.entries) != 0 {
		t.Fatalf("invalidated flight repopulated cache: %v %#v", err, cache.entries)
	}
	for _, share := range []bool{true, false} {
		_, err := cache.do(context.Background(), "key", "owner", func(*UploadFileResult) (*UploadFileResult, bool, error) {
			return &UploadFileResult{ID: "failed-file", Status: "FAILED"}, share, nil
		})
		if err != nil || len(cache.entries) != 0 {
			t.Fatal("failed upload was cached")
		}
	}
}

func TestUploadCacheCanceledWaiterDoesNotCancelLeader(t *testing.T) {
	cache := newUploadReuseCache(time.Minute, 2)
	leaderCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, release := make(chan struct{}), make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		_, err := cache.do(leaderCtx, "key", "owner", func(*UploadFileResult) (*UploadFileResult, bool, error) {
			close(started)
			select {
			case <-release:
				return &UploadFileResult{ID: "leader-file", Status: "SUCCESS"}, true, nil
			case <-leaderCtx.Done():
				return nil, false, leaderCtx.Err()
			}
		})
		finished <- err
	}()
	<-started
	waiterCtx, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer stop()
	_, err := cache.do(waiterCtx, "key", "owner", func(*UploadFileResult) (*UploadFileResult, bool, error) {
		return nil, false, errors.New("waiter unexpectedly started another upload")
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiter did not cancel: %v", err)
	}
	close(release)
	if err := <-finished; err != nil || cache.entries["key"].file == nil {
		t.Fatalf("waiter canceled successful leader: %v", err)
	}
}

func TestUploadCacheConfiguration(t *testing.T) {
	t.Setenv("DS2API_UPLOAD_CACHE_TTL_SECONDS", "0")
	if newUploadReuseCacheFromEnv() != nil {
		t.Fatal("zero TTL did not disable reuse")
	}
	t.Setenv("DS2API_UPLOAD_CACHE_TTL_SECONDS", "invalid")
	t.Setenv("DS2API_UPLOAD_CACHE_MAX_ENTRIES", "999999")
	cache := newUploadReuseCacheFromEnv()
	if cache.ttl != 15*time.Minute || cache.limit != 4096 {
		t.Fatalf("configuration not bounded: %#v", cache)
	}
}
