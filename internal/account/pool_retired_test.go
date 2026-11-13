package account

import (
	"ds2api/internal/config"
	"path/filepath"
	"testing"
)

func TestRegressionDeletingBusyAccountMustRetainGlobalSlot(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"accounts":[{"email":"a@example.com","password":"pwd"},{"email":"b@example.com","password":"pwd"}],"runtime":{"global_max_inflight":1,"account_max_inflight":1,"account_rate_limit":{"enabled":false}}}`)
	t.Setenv("DS2API_ENV_WRITEBACK", "")
	t.Setenv("DS2API_ACCOUNT_DAILY_LIMIT_PATH", filepath.Join(t.TempDir(), "daily.json"))
	s, err := config.LoadStoreWithError()
	if err != nil {
		t.Fatal(err)
	}
	p := NewPool(s)
	defer p.Close()
	a, ok := p.Acquire("a@example.com", nil, nil)
	if !ok {
		t.Fatal("could not acquire A")
	}
	defer p.Release(a.Identifier())
	if err = s.Update(func(c *config.Config) error { c.Accounts = c.Accounts[1:]; return nil }); err != nil {
		t.Fatal(err)
	}
	p.Reset()
	b, ok := p.Acquire("b@example.com", nil, nil)
	if ok {
		p.Release(b.Identifier())
		t.Fatal("admitted B while unreleased A still runs, exceeding global_max_inflight=1")
	}
}
