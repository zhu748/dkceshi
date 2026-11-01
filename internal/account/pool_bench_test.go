package account

import (
	"encoding/json"
	"fmt"
	"testing"

	"ds2api/internal/config"
)

func BenchmarkAcquireSaturatedGlobalLimit(b *testing.B) {
	cfg := config.Config{Keys: []string{"key"}}
	for i := range 5000 {
		cfg.Accounts = append(cfg.Accounts, config.Account{Email: fmt.Sprintf("account-%d@example.com", i), Token: "token"})
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		b.Fatal(err)
	}
	b.Setenv("DS2API_CONFIG_JSON", string(raw))
	pool := NewPool(config.LoadStore())
	b.Cleanup(pool.Close)
	pool.ApplyRuntimeLimits(1, 5000, 1)
	if _, ok := pool.Acquire("", nil, nil); !ok {
		b.Fatal("occupy the global slot")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, ok := pool.Acquire("", nil, nil); ok {
			b.Fatal("global limit bypassed")
		}
	}
}

func BenchmarkNotifyBlockedWaiters(b *testing.B) {
	cfg := config.Config{Keys: []string{"key"}}
	for i := range 5000 {
		cfg.Accounts = append(cfg.Accounts, config.Account{Email: fmt.Sprintf("busy-%d@example.com", i), Token: "token"})
	}
	cfg.Accounts = append(cfg.Accounts, config.Account{Email: "tools@example.com", Token: "token", PoolType: config.PoolTypeToolsOnly})
	raw, err := json.Marshal(cfg)
	if err != nil {
		b.Fatal(err)
	}
	b.Setenv("DS2API_CONFIG_JSON", string(raw))
	p := NewPool(config.LoadStore())
	b.Cleanup(p.Close)
	p.ApplyRuntimeLimits(1, 200, 5001)
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range 5000 {
		p.inUse[fmt.Sprintf("busy-%d@example.com", i)] = 1
	}
	p.inUseTotal = 5000
	for range 200 {
		p.waiters = append(p.waiters, &poolWaiter{ready: make(chan struct{}), filter: func(acc config.Account) bool { return acc.MatchesPoolType(false) }})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		p.notifyWaiterLocked()
	}
	if len(p.waiters) != 200 {
		b.Fatal("blocked waiters were incorrectly woken")
	}
}
