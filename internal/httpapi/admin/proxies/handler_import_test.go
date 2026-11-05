package proxies

import (
	"bytes"
	"ds2api/internal/config"
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func proxyRouter(h *Handler) http.Handler { r := chi.NewRouter(); RegisterRoutes(r, h); return r }
func proxyRequest(t *testing.T, r http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
	return w
}
func TestImportPreservesCredentialsOptionsAndBindings(t *testing.T) {
	h := newAdminProxyTestHandler(t, `{"proxies":[{"id":"old","type":"turns","host":"relay.example","port":5349,"username":"u","password":"p"}],"accounts":[{"email":"a@example.com","proxy_id":"old"}]}`)
	r := proxyRouter(h)
	preview := proxyRequest(t, r, "POST", "/proxies/parse", `{"type":"sstp","text":"u:p@vpn.example:443\nturns://u:p@relay.example:5349\nturns://u:changed@relay.example:5349?sni=cert.example\nturn:bad.example?transport=udp"}`)
	if preview.Code != 200 {
		t.Fatal(preview.Body.String())
	}
	var data struct {
		Items []struct {
			Status string       `json:"status"`
			Proxy  config.Proxy `json:"proxy"`
		}
	}
	if e := json.Unmarshal(preview.Body.Bytes(), &data); e != nil {
		t.Fatal(e)
	}
	if len(data.Items) != 4 || data.Items[0].Status != "ready" || data.Items[1].Status != "duplicate" || data.Items[2].Status != "ready" || data.Items[3].Status != "invalid" {
		t.Fatalf("unexpected preview: %s", preview.Body.String())
	}
	items := []config.Proxy{data.Items[0].Proxy, data.Items[1].Proxy, data.Items[2].Proxy, data.Items[0].Proxy}
	body, e := json.Marshal(map[string]any{"items": items})
	if e != nil {
		t.Fatal(e)
	}
	w := proxyRequest(t, r, "POST", "/proxies/import", string(body))
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"added":2`)) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	snapshot := h.Store.Snapshot()
	if len(snapshot.Proxies) != 3 || snapshot.Accounts[0].ProxyID != "old" {
		t.Fatalf("bindings overwritten: %#v", snapshot.Proxies)
	}
	list := proxyRequest(t, r, "GET", "/proxies", "")
	if strings.Contains(list.Body.String(), "changed") || strings.Contains(list.Body.String(), `"password"`) {
		t.Fatal("password exposed")
	}
	if !strings.Contains(list.Body.String(), "cert.example") {
		t.Fatal("TLS options not returned")
	}
	// Import validates every item before committing any of them.
	bad := proxyRequest(t, r, "POST", "/proxies/import", `{"items":[{"type":"http","host":"good.example","port":80},{"type":"bad","host":"bad.example","port":80}]}`)
	if bad.Code != 400 || len(h.Store.Snapshot().Proxies) != 3 {
		t.Fatal("invalid import partially committed")
	}
}
func TestExplicitPasswordClearAndStrictRequests(t *testing.T) {
	h := newAdminProxyTestHandler(t, `{"proxies":[{"id":"p","type":"socks5","host":"localhost","port":1080,"username":"u","password":"secret"}]}`)
	r := proxyRouter(h)
	for _, suffix := range []string{`"password":""`, `"password":"","clear_password":true`} {
		w := proxyRequest(t, r, "PUT", "/proxies/p", `{"type":"socks5","host":"localhost","port":1080,"username":"u",`+suffix+`}`)
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		got := h.Store.Snapshot().Proxies[0].Password
		if strings.Contains(suffix, "clear_password") {
			if got != "" {
				t.Fatal("clear ignored")
			}
		} else if got != "secret" {
			t.Fatal("blank did not preserve saved password")
		}
	}
	for _, body := range []string{`{"type":"http","host":"one","host":"two","port":80}`, `{"type":"http","host":"one","port":80.1}`, `{"type":"http","host":"one","port":80} {}`, `{"proxy_id":123}`} {
		w := proxyRequest(t, r, "POST", "/proxies", body)
		if w.Code != 400 {
			t.Fatalf("accepted %s: %s", body, w.Body.String())
		}
	}
}

func TestSingleAddSkipsDuplicateWithoutOverwritingName(t *testing.T) {
	h := newAdminProxyTestHandler(t, `{"proxies":[{"id":"existing","name":"original","type":"turn","host":"relay.example","port":3478}]}`)
	w := proxyRequest(t, proxyRouter(h), "POST", "/proxies", `{"name":"renamed","type":"turn","host":"relay.example","port":3478}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"skipped":true`) {
		t.Fatal(w.Body.String())
	}
	p := h.Store.Snapshot().Proxies
	if len(p) != 1 || p[0].ID != "existing" || p[0].Name != "original" {
		t.Fatal("duplicate overwrote existing node")
	}
}

func TestRejectInvalidDuplicateAndNonObjectRequests(t *testing.T) {
	h := newAdminProxyTestHandler(t, `{"proxies":[{"id":"p","type":"sstp","host":"vpn.example","port":443,"username":"u","password":"p"}],"accounts":[{"email":"a@example.com","proxy_id":"p"}]}`)
	r := proxyRouter(h)
	body := `{"items":[{"type":"sstp","host":"vpn.example","port":443,"username":"u","password":"p","mtu":99999}]}`
	if w := proxyRequest(t, r, "POST", "/proxies/import", body); w.Code != 400 {
		t.Fatalf("invalid duplicate skipped validation: %s", w.Body.String())
	}
	for _, tc := range []struct{ method, path, body string }{{"POST", "/proxies", "null"}, {"PUT", "/accounts/a@example.com/proxy", "null"}, {"POST", "/proxies/import", `{"items":[],"unknown":true}`}, {"POST", "/proxies", `{"type":"http","host":"relay.example","port":80,"typo":"ignored"}`}} {
		w := proxyRequest(t, r, tc.method, tc.path, tc.body)
		if w.Code != 400 {
			t.Fatalf("accepted malformed request %s %s: %s", tc.path, tc.body, w.Body.String())
		}
	}
	if h.Store.Snapshot().Accounts[0].ProxyID != "p" {
		t.Fatal("malformed request unbound account")
	}
}

func TestPreviewAllowsJSONEscapingOverheadWithinTextLimit(t *testing.T) {
	h := newAdminProxyTestHandler(t, `{"accounts":[]}`)
	node, err := json.Marshal(map[string]any{"type": "turn", "host": "relay.example", "name": strings.Repeat("\"", 100000)})
	if err != nil {
		t.Fatal(err)
	}
	request, err := json.Marshal(map[string]any{"text": string(node), "type": "turn"})
	if err != nil {
		t.Fatal(err)
	}
	if len(node) > 256*1024 || len(request) <= 256*1024+4096 {
		t.Fatal("fixture does not cover escaped input size boundary")
	}
	w := proxyRequest(t, proxyRouter(h), "POST", "/proxies/parse", string(request))
	if w.Code != 200 {
		t.Fatalf("valid input text rejected due to envelope size: %s", w.Body.String())
	}
}
