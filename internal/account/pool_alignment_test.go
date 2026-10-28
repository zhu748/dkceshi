package account

import (
	"fmt"
	"testing"
	"time"

	"ds2api/internal/config"
)

func TestElasticRespectsManualDisableAndCooldown(t *testing.T) {
	cfg := config.Config{ElasticPool: config.ElasticPoolConfig{Enabled: true, GlobalCount: 1}, Accounts: []config.Account{
		{Email: "manual@example.com", ManualDisabled: true},
		{Email: "cooling@example.com", CooldownUntil: float64(time.Now().Unix() + 60)},
		{Email: "ready@example.com"},
	}}
	ReconcileElasticPool(&cfg)
	if !cfg.Accounts[0].Disabled || !cfg.Accounts[1].Disabled || cfg.Accounts[2].Disabled {
		t.Fatal("unavailable account consumed elastic slot")
	}
	DisableAllAccounts(&cfg)
	if !cfg.Accounts[0].Disabled {
		t.Fatal("turning off elastic pool re-enabled manually disabled account")
	}
}

func TestPoolRebuildPreservesQuotasWhileDisabled(t *testing.T) {
	pool, store, _ := newDailyLimitPool(t, `{"enabled":true,"max_per_day":1}`, "one@example.com")
	defer pool.Close()
	if !acquireOnce(t, pool) {
		t.Fatal("first acquire")
	}
	for _, disabled := range []bool{true, false} {
		if err := store.Update(func(c *config.Config) error { c.Accounts[0].Disabled = disabled; return nil }); err != nil {
			t.Fatal(err)
		}
		pool.Reset()
	}
	if acquireOnce(t, pool) {
		t.Fatal("queue rebuild granted another daily quota")
	}
}

func TestElasticPoolRecoversOnExpiryAndStartup(t *testing.T) {
	until := time.Now().Unix() + 1
	t.Setenv("DS2API_CONFIG_JSON", fmt.Sprintf(`{"elastic_pool":{"enabled":true,"global_count":1},"accounts":[{"email":"first@example.com","muted_until":%d,"disabled":true},{"email":"second@example.com"}]}`, until))
	store := config.LoadStore()
	pool := NewPool(store)
	defer pool.Close()
	if first, _ := store.FindAccount("first@example.com"); !first.Disabled {
		t.Fatal("mute not respected")
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		first, _ := store.FindAccount("first@example.com")
		second, _ := store.FindAccount("second@example.com")
		if !first.Disabled && second.Disabled {
			if pool.SchedulableCount() != 1 {
				t.Fatal("queue did not recover")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expired mute did not restore elastic priority")
}

func TestQueueRebuildPreservesMinuteQuota(t *testing.T) {
	pool, store := newRateLimitedPoolForTest(t, true, 1, "one@example.com")
	defer pool.Close()
	if !acquireOnce(t, pool) {
		t.Fatal("first acquire")
	}
	if err := store.Update(func(c *config.Config) error { c.Accounts[0].Remark = "changed"; return nil }); err != nil {
		t.Fatal(err)
	}
	pool.Reset()
	if acquireOnce(t, pool) {
		t.Fatal("queue rebuild cleared minute quota")
	}
}
