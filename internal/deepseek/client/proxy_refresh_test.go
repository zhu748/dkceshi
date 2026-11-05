package client

import (
	"ds2api/internal/config"
	"testing"
)

func TestProxyRefreshPreservesUnchangedPoolAndDropsChangedNode(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"proxies":[{"id":"one","type":"socks5h","host":"one.example","port":1080},{"id":"two","type":"socks5h","host":"two.example","port":1080}]}`)
	c := NewClient(config.LoadStore(), nil)
	defer c.Close()
	first := c.requestClientsForAccount(config.Account{ProxyID: "one"}).regular
	c.requestClientsForAccount(config.Account{ProxyID: "two"})
	if err := c.Store.Update(func(cfg *config.Config) error {
		cfg.Proxies[0].Name = "renamed"
		cfg.Proxies[1].Password = "changed"
		cfg.Proxies[1].Username = "user"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	c.RefreshProxyClients()
	if c.requestClientsForAccount(config.Account{ProxyID: "one"}).regular != first {
		t.Fatal("unrelated node lost its pool")
	}
	c.proxyClientsMu.RLock()
	n := len(c.proxyClients)
	c.proxyClientsMu.RUnlock()
	if n != 1 {
		t.Fatalf("stale node pool retained: %d", n)
	}
}
