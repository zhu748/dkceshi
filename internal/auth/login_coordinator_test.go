package auth

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ds2api/internal/config"
)

func TestConcurrentAccountRefreshesShareLogin(t *testing.T) {
	r := newTestResolver(t)
	t.Cleanup(r.Pool.Close)
	acc := r.Store.Accounts()[0]
	var calls atomic.Int32
	entered, release := make(chan struct{}), make(chan struct{})
	r.Login = func(context.Context, config.Account) (string, error) {
		if calls.Add(1) == 1 {
			close(entered)
		}
		<-release
		return "refreshed", nil
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			a := r.NewKeepAliveAuth(acc)
			if err := r.RefreshTokenErr(context.Background(), a); err != nil {
				t.Error(err)
			}
			if a.DeepSeekToken != "refreshed" {
				t.Errorf("token not shared: %q", a.DeepSeekToken)
			}
		})
	}
	<-entered
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("duplicate logins: %d", calls.Load())
	}
	stored, _ := r.Store.FindAccount(acc.Identifier())
	if stored.Token != "refreshed" {
		t.Fatal("a stale refresher cleared the shared token")
	}
}

func TestCancelledRefreshWaiterDoesNotCancelLeader(t *testing.T) {
	r := newTestResolver(t)
	t.Cleanup(r.Pool.Close)
	acc := r.Store.Accounts()[0]
	entered, release := make(chan struct{}), make(chan struct{})
	r.Login = func(context.Context, config.Account) (string, error) { close(entered); <-release; return "fresh", nil }
	result := make(chan error, 1)
	go func() { result <- r.RefreshTokenErr(context.Background(), r.NewKeepAliveAuth(acc)) }()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err := r.RefreshTokenErr(ctx, r.NewKeepAliveAuth(acc))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiter cancellation lost: %v", err)
	}
	close(release)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func TestRefreshWaiterRecoversFromCancelledLeader(t *testing.T) {
	r := newTestResolver(t)
	t.Cleanup(r.Pool.Close)
	acc := r.Store.Accounts()[0]
	var calls atomic.Int32
	entered := make(chan struct{})
	r.Login = func(ctx context.Context, _ config.Account) (string, error) {
		if calls.Add(1) == 1 {
			close(entered)
			<-ctx.Done()
			return "", ctx.Err()
		}
		return "recovered", nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() { first <- r.RefreshTokenErr(ctx, r.NewKeepAliveAuth(acc)) }()
	<-entered
	cancel()
	a := r.NewKeepAliveAuth(acc)
	if err := r.RefreshTokenErr(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Fatalf("leader cancellation lost: %v", err)
	}
	if calls.Load() != 2 || a.DeepSeekToken != "recovered" {
		t.Fatalf("live waiter was poisoned: calls=%d token=%q", calls.Load(), a.DeepSeekToken)
	}
}

func TestPanickedLoginDoesNotStrandTheAccount(t *testing.T) {
	r := newTestResolver(t)
	t.Cleanup(r.Pool.Close)
	acc := r.Store.Accounts()[0]
	r.Login = func(context.Context, config.Account) (string, error) { panic("login failure") }
	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected original panic to reach request recovery")
			}
		}()
		_ = r.RefreshTokenErr(context.Background(), r.NewKeepAliveAuth(acc))
	}()
	r.mu.Lock()
	remaining := len(r.loginFlights)
	r.mu.Unlock()
	if remaining != 0 {
		t.Fatal("panic left a permanently pending login")
	}
	r.Login = func(context.Context, config.Account) (string, error) { return "recovered", nil }
	if err := r.RefreshTokenErr(context.Background(), r.NewKeepAliveAuth(acc)); err != nil {
		t.Fatal(err)
	}
}
