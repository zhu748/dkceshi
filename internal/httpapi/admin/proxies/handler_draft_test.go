package proxies

import (
	"context"
	"ds2api/internal/config"
	"testing"
)

func TestDraftConnectivityUsesSavedPasswordWithoutSavingDraft(t *testing.T) {
	h := newAdminProxyTestHandler(t, `{"proxies":[{"id":"p","type":"socks5h","host":"old.example","port":1080,"username":"user","password":"private"}]}`)
	old := proxyConnectivityTester
	defer func() { proxyConnectivityTester = old }()
	var tested config.Proxy
	proxyConnectivityTester = func(_ context.Context, p config.Proxy) map[string]any {
		tested = p
		return map[string]any{"success": true}
	}
	r := proxyRouter(h)
	for _, tc := range []struct{ clear, want string }{{"false", "private"}, {"true", ""}} {
		w := proxyRequest(t, r, "POST", "/proxies/test", `{"base_proxy_id":"p","type":"socks5h","host":"draft.example","port":1081,"username":"user","password":"","clear_password":`+tc.clear+`}`)
		if w.Code != 200 || tested.Password != tc.want || tested.Host != "draft.example" {
			t.Fatalf("draft request %d: %+v", w.Code, tested)
		}
		p, _ := findProxyByID(h.Store.Snapshot(), "p")
		if p.Host != "old.example" || p.Password != "private" {
			t.Fatal("testing persisted the draft")
		}
	}
}
