package client

import (
	"context"
	"ds2api/internal/config"
	"net/http"
	"path/filepath"
	"testing"
)

func TestBrokenAccountProxyDoesNotUseDefaultClients(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"proxies":[{"id":"p","type":"https","host":"proxy.invalid","port":443,"ca_file":"missing-proxy-ca.pem"}]}`)
	store := config.LoadStore()
	c := NewClient(store, nil)
	defer c.Close()
	calls := 0
	c.regular = alignmentDoer(func(*http.Request) (*http.Response, error) {
		calls++
		t.Error("default route used after proxy failure")
		return nil, nil
	})
	req, err := http.NewRequestWithContext(context.Background(), "GET", "https://target.invalid", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"p", "missing"} {
		bundle := c.requestClientsForAccount(config.Account{ProxyID: id})
		if _, err := bundle.regular.Do(req); err == nil {
			t.Fatal("unavailable proxy silently succeeded")
		}
	}
	if calls != 0 {
		t.Fatal("default client used")
	}
}
func TestProxyCacheIdentityIncludesTLSAndCredentials(t *testing.T) {
	p := config.Proxy{ID: "p", Type: "https", Host: "proxy.invalid", Port: 443, Username: "u", Password: "p"}
	key := proxyCacheKey(p)
	variants := []config.Proxy{p, p, p, p, p}
	variants[0].ServerName = "other.invalid"
	variants[1].CAFile = filepath.Join("certs", "ca.pem")
	variants[2].Insecure = true
	variants[3].Password = "changed"
	variants[4].Username = "changed"
	for _, v := range variants {
		if proxyCacheKey(v) == key {
			t.Fatal("connection settings share a cached client")
		}
	}
	p.Name = "renamed"
	if proxyCacheKey(p) != key {
		t.Fatal("renaming unnecessarily changed connection identity")
	}
}
