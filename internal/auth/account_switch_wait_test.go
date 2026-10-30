package auth

import (
	"context"
	"testing"
	"time"

	"ds2api/internal/account"
	"ds2api/internal/config"
)

func newSwitchWaitResolver(t *testing.T) (*Resolver, *account.Pool) {
	t.Helper()
	t.Setenv("DS2API_CONFIG_JSON", `{"keys":["managed-key"],"runtime":{"account_max_inflight":1,"account_max_queue":2},"accounts":[{"email":"a@example.com","token":"ta"},{"email":"b@example.com","token":"tb"}]}`)
	store := config.LoadStore()
	pool := account.NewPool(store)
	r := NewResolver(store, pool, func(_ context.Context, acc config.Account) (string, error) { return "token-" + acc.Identifier(), nil })
	return r, pool
}

func TestSwitchAccountWaitsForBusyReplacement(t *testing.T) {
	r, pool := newSwitchWaitResolver(t)
	a, busy := determineManaged(t, r), determineManaged(t, r)
	defer r.Release(a)
	defer r.Release(busy)
	oldID, replacement := a.AccountID, busy.AccountID
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result := make(chan bool, 1)
	go func() { result <- a.SwitchAccount(ctx) }()
	deadline := time.Now().Add(time.Second)
	for pool.Status()["waiting"] != 1 {
		if time.Now().After(deadline) {
			t.Fatal("failover did not enter the waiting queue")
		}
		time.Sleep(time.Millisecond)
	}
	r.Release(busy)
	if !<-result || a.AccountID != replacement || a.AccountID == oldID {
		t.Fatal("failover did not acquire the freed replacement")
	}
	r.Release(a)
	if pool.Status()["in_use"] != 0 {
		t.Fatal("failover leaked or double-counted an account slot")
	}
}

func TestSwitchAccountWaitHonorsCancellationAndReleasesOnce(t *testing.T) {
	r, pool := newSwitchWaitResolver(t)
	a, busy := determineManaged(t, r), determineManaged(t, r)
	defer r.Release(busy)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if a.SwitchAccount(ctx) {
		t.Fatal("busy replacement should not be acquired after deadline")
	}
	r.Release(a)
	if pool.Status()["in_use"] != 1 || pool.Status()["waiting"] != 0 {
		t.Fatalf("incorrect accounting after cancelled failover: %+v", pool.Status())
	}
}
