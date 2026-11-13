package clientaddr

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHeaderRewritesCannotChangeThrottleIdentityWithoutExplicitTrust(t *testing.T) {
	for _, trusted := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "trusted"}[trusted], func(t *testing.T) {
			t.Setenv("DS2API_TRUST_PROXY_HEADERS", map[bool]string{false: "", true: "1"}[trusted])
			req := httptest.NewRequest("POST", "/admin/login", nil)
			req.RemoteAddr = "192.0.2.1:1234"
			h := Capture(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				r.RemoteAddr = "198.51.100.8"
				want := "192.0.2.1"
				if trusted {
					want = "198.51.100.8"
				}
				if got := IP(r); got != want {
					t.Fatalf("got %s want %s", got, want)
				}
			}))
			h.ServeHTTP(httptest.NewRecorder(), req)
		})
	}
}
