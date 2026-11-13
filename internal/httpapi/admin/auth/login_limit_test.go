package auth

import (
	authn "ds2api/internal/auth"
	"ds2api/internal/config"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLoginBudgetRefillsAndMemoryIsBounded(t *testing.T) {
	var limiter loginLimiter
	now := time.Now()
	for i := 0; i < loginBurst; i++ {
		if ok, _ := limiter.allow("peer", now); !ok {
			t.Fatal("initial budget missing")
		}
	}
	if ok, retry := limiter.allow("peer", now); ok || retry != 30 {
		t.Fatalf("budget not enforced: %v %d", ok, retry)
	}
	if ok, _ := limiter.allow("peer", now.Add(loginRefill)); !ok {
		t.Fatal("budget did not refill")
	}
	for i := 0; i < maxLoginPeers+5; i++ {
		limiter.allow(fmt.Sprint(i), now)
	}
	if len(limiter.peers) > maxLoginPeers {
		t.Fatal("unbounded limiter memory")
	}
}
func TestLoginRespondsWithRetryAfterAndRejectsLargePayloads(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{}`)
	h := &Handler{Store: config.LoadStore()}
	for i := 0; i < loginBurst; i++ {
		rec := httptest.NewRecorder()
		h.login(rec, httptest.NewRequest("POST", "/admin/login", strings.NewReader(`{"admin_key":"wrong"}`)))
	}
	rec := httptest.NewRecorder()
	h.login(rec, httptest.NewRequest("POST", "/admin/login", strings.NewReader(`{"admin_key":"wrong"}`)))
	if rec.Code != 429 || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("missing throttle: %d", rec.Code)
	}
	h = &Handler{Store: config.LoadStore()}
	rec = httptest.NewRecorder()
	h.login(rec, httptest.NewRequest("POST", "/admin/login", strings.NewReader(`{"admin_key":"`+strings.Repeat("x", 17<<10)+`"}`)))
	if rec.Code != 413 {
		t.Fatalf("oversized login accepted: %d", rec.Code)
	}
}

func TestProtectedPasswordAuthSharesFailureBudgetButSignedSessionsStayAvailable(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{}`)
	t.Setenv("DS2API_ADMIN_KEY", "test-password")
	h := &Handler{Store: config.LoadStore()}
	endpoint := h.requireAdmin(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	call := func(token string) int {
		req := httptest.NewRequest("GET", "/admin/config", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		endpoint.ServeHTTP(rec, req)
		return rec.Code
	}
	for i := 0; i < loginBurst+2; i++ {
		if got := call("test-password"); got != 204 {
			t.Fatalf("valid raw password throttled at %d: %d", i, got)
		}
	}
	for i := 0; i < loginBurst; i++ {
		if got := call("wrong-password"); got != 401 {
			t.Fatalf("unexpected status at %d: %d", i, got)
		}
	}
	if got := call("wrong-password"); got != 429 {
		t.Fatalf("password guessing bypasses login budget: %d", got)
	}
	token, err := authn.CreateJWTWithStore(1, h.Store)
	if err != nil {
		t.Fatal(err)
	}
	if got := call(token); got != 204 {
		t.Fatalf("valid signed session was locked out: %d", got)
	}
}
