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
