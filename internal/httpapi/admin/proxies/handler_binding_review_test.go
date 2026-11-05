package proxies

import (
	"strings"
	"testing"
)

func TestBindingRequiresAnExplicitIntent(t *testing.T) {
	h := newAdminProxyTestHandler(t, `{"proxies":[{"id":"p","type":"socks5h","host":"relay.example","port":1080}],"accounts":[{"email":"a@example.com","proxy_id":"p","manual_binding":true}]}`)
	r := proxyRouter(h)
	for _, body := range []string{`{}`, `{"mode":""}`, `{"mode":"invalid"}`, `{"mode":"auto","proxy_id":"p"}`, `{"host":"ignored.example"}`, `{"name":"ignored"}`} {
		w := proxyRequest(t, r, "PUT", "/accounts/a@example.com/proxy", body)
		if w.Code != 400 {
			t.Fatalf("ambiguous binding request accepted: %s => %s", body, w.Body.String())
		}
		account, _ := h.Store.FindAccount("a@example.com")
		if account.ProxyID != "p" || !account.ManualBinding {
			t.Fatal("rejected request changed the binding")
		}
	}
	w := proxyRequest(t, r, "POST", "/proxies/test", `{"proxy_id":"p","host":"other.example"}`)
	if w.Code != 400 {
		t.Fatalf("saved/temporary proxy test ambiguity accepted: %s", w.Body.String())
	}
}
func TestBridgeOwnedProxiesCannotBeMutatedThroughManualCRUD(t *testing.T) {
	h := newAdminProxyTestHandler(t, `{"proxies":[{"id":"mihomo-owned","type":"socks5","host":"127.0.0.1","port":10801}]}`)
	r := proxyRouter(h)
	for _, method := range []string{"PUT", "DELETE"} {
		w := proxyRequest(t, r, method, "/proxies/mihomo-owned", `{"type":"turn","host":"relay.example","port":3478}`)
		if w.Code != 409 {
			t.Fatalf("managed %s accepted: %d %s", method, w.Code, w.Body.String())
		}
	}
	w := proxyRequest(t, r, "POST", "/proxies", `{"id":"mihomo-forged","type":"http","host":"relay.example","port":80}`)
	if w.Code != 400 {
		t.Fatalf("reserved ID accepted: %s", w.Body.String())
	}
	if !strings.Contains(h.Store.Snapshot().Proxies[0].Host, "127.0.0.1") {
		t.Fatal("bridge node overwritten")
	}
}
