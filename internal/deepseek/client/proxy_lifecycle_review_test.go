package client

import (
	"context"
	"ds2api/internal/config"
	trans "ds2api/internal/deepseek/transport"
	"errors"
	"net"
	"net/http"
	"testing"
)

func TestCallerCancellationDoesNotPenalizeNode(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", "https://target.invalid", nil)
	if err != nil {
		t.Fatal(err)
	}
	reports := 0
	d := reportingDoer{inner: alignmentDoer(func(*http.Request) (*http.Response, error) { return nil, context.Canceled }), id: "p", report: func(string, bool) { reports++ }}
	if _, err = d.Do(req); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if reports != 0 {
		t.Fatal("caller cancellation was counted as a proxy failure")
	}
	live, err := http.NewRequest("GET", "https://target.invalid", nil)
	if err != nil {
		t.Fatal(err)
	}
	d.inner = alignmentDoer(func(*http.Request) (*http.Response, error) { return nil, errors.New("connection refused") })
	if _, err = d.Do(live); err == nil {
		t.Fatal("network failure swallowed")
	}
	if reports != 1 {
		t.Fatal("real proxy failure not reported")
	}
}
func TestClosedClientCannotRecreateCachedOrDefaultTransports(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"proxies":[{"id":"p","type":"socks5h","host":"localhost","port":1080}]}`)
	c := NewClient(config.LoadStore(), nil)
	c.requestClientsForAccount(config.Account{ProxyID: "p"})
	c.Close()
	c.ResetProxyClients()
	req, _ := http.NewRequest("GET", "https://target.invalid", nil)
	for _, account := range []config.Account{{ProxyID: "p"}, {NoProxy: true}, {}} {
		b := c.requestClientsForAccount(account)
		if _, ok := b.regular.(failedProxyDoer); !ok {
			t.Fatal("closed client returned a live transport")
		}
		if _, err := b.regular.Do(req); err == nil {
			t.Fatal("closed client accepted requests")
		}
	}
	if _, ok := c.defaultRequestClients().regular.(failedProxyDoer); !ok {
		t.Fatal("default transport survived close")
	}
	c.proxyClientsMu.RLock()
	n := len(c.proxyClients)
	c.proxyClientsMu.RUnlock()
	if n != 0 {
		t.Fatal("cache repopulated after close")
	}
}

func TestResetDuringBuildDoesNotReinsertOldProxyConfiguration(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"proxies":[{"id":"p","type":"socks5h","host":"old.example","port":1080}]}`)
	store := config.LoadStore()
	c := NewClient(store, nil)
	defer c.Close()
	started, release := make(chan struct{}), make(chan struct{})
	done := make(chan requestClients, 1)
	calls := 0
	build := func(p config.Proxy) (trans.DialContextFunc, error) {
		calls++
		if calls == 1 {
			close(started)
			<-release
		}
		return (&net.Dialer{}).DialContext, nil
	}
	go func() { done <- c.requestClientsWithDialer(config.Account{ProxyID: "p"}, build) }()
	<-started
	if err := store.Update(func(cfg *config.Config) error { cfg.Proxies[0].Host = "new.example"; return nil }); err != nil {
		t.Fatal(err)
	}
	c.ResetProxyClients()
	close(release)
	<-done
	current, _ := store.ProxyByID("p")
	key := proxyCacheKey(current)
	c.proxyClientsMu.RLock()
	_, ok := c.proxyClients[key]
	n := len(c.proxyClients)
	c.proxyClientsMu.RUnlock()
	if !ok || n != 1 || calls != 2 {
		t.Fatalf("reset race retained stale settings: builds=%d entries=%d", calls, n)
	}
}
func TestCloseDuringBuildCannotPublishANewPool(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"proxies":[{"id":"p","type":"socks5h","host":"relay.example","port":1080}]}`)
	c := NewClient(config.LoadStore(), nil)
	started, release := make(chan struct{}), make(chan struct{})
	done := make(chan requestClients, 1)
	go func() {
		done <- c.requestClientsWithDialer(config.Account{ProxyID: "p"}, func(config.Proxy) (trans.DialContextFunc, error) {
			close(started)
			<-release
			return (&net.Dialer{}).DialContext, nil
		})
	}()
	<-started
	c.Close()
	close(release)
	b := <-done
	if _, ok := b.regular.(failedProxyDoer); !ok {
		t.Fatal("closed client published a pool from an unfinished build")
	}
	c.proxyClientsMu.RLock()
	n := len(c.proxyClients)
	c.proxyClientsMu.RUnlock()
	if n != 0 {
		t.Fatal("close race left a cached pool")
	}
}
