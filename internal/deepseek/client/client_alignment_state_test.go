package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"ds2api/internal/account"
	"ds2api/internal/auth"
	"ds2api/internal/config"
)

type alignmentDoer func(*http.Request) (*http.Response, error)

func (f alignmentDoer) Do(r *http.Request) (*http.Response, error) { return f(r) }

type alignmentBody struct {
	io.Reader
	closed bool
}

func (b *alignmentBody) Close() error { b.closed = true; return nil }

func TestLoginReusesStoredAndroidIdentityAndClassifiesRejectedCredentials(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"accounts":[{"email":"login@example.com","password":"p","device_model":"Pixel 9a","android_api_level":"36","device_id":"sdk-risk-id","installation_id":"fixed-android-id"}]}`)
	store := config.LoadStore()
	c := NewClient(store, nil)
	defer c.Close()
	rejected := false
	c.regular = alignmentDoer(func(r *http.Request) (*http.Response, error) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		wantBodyID := "fixed-android-id"
		if r.URL.Path == "/api/v0/users/login" {
			wantBodyID = "sdk-risk-id"
		}
		if body["device_id"] != wantBodyID || r.Header.Get("x-device-id") != "fixed-android-id" || r.Header.Get("x-device-model") != "Pixel 9a" || !strings.Contains(r.Header.Get("User-Agent"), "Android/36") {
			t.Fatal("login body and request identity disagree")
		}
		raw := `{"code":0,"data":{"biz_code":0,"biz_data":{"user":{"token":"new-token"}}}}`
		if rejected {
			raw = `{"code":0,"data":{"biz_code":1,"biz_msg":"invalid password"}}`
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(raw))}, nil
	})
	acc := config.Account{Email: "login@example.com", Password: "p"}
	if tok, err := c.Login(context.Background(), acc); err != nil || tok != "new-token" {
		t.Fatalf("login failed: %v", err)
	}
	rejected = true
	if _, err := c.Login(context.Background(), acc); !errors.Is(err, auth.ErrLoginRejected) {
		t.Fatalf("login rejection not classified: %v", err)
	}
}

func TestCompletionMutePersistsAndSSEInspectionPreservesClose(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"accounts":[{"email":"muted@example.com","password":"p"}]}`)
	store := config.LoadStore()
	pool := account.NewPool(store)
	defer pool.Close()
	resolver := auth.NewResolver(store, pool, nil)
	c := NewClient(store, resolver)
	defer c.Close()
	// No prefetches are needed for this test; the upstream is entirely local.
	c.powCache = nil
	body := &alignmentBody{Reader: strings.NewReader(`{"code":0,"data":{"biz_code":5,"biz_msg":"user is muted"}}`)}
	c.stream = alignmentDoer(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: body}, nil
	})
	a := &auth.RequestAuth{AccountID: "muted@example.com", Account: config.Account{Email: "muted@example.com"}, UseConfigToken: true, DeepSeekToken: "tok"}
	if _, err := c.CallCompletion(context.Background(), a, map[string]any{}, "pow", 1); !IsMutedError(err) {
		t.Fatalf("mute not surfaced: %v", err)
	}
	got, _ := store.FindAccount(a.AccountID)
	if !got.IsMuted() || !body.closed || pool.SchedulableCount() != 0 {
		t.Fatal("mute state did not reach scheduling/cleanup")
	}
	sse := &alignmentBody{Reader: strings.NewReader("data: [DONE]\n\n")}
	restored, muted, _, err := detectMutedCompletion(sse)
	if err != nil || muted {
		t.Fatal("SSE inspection failed")
	}
	raw, err := io.ReadAll(restored)
	if err != nil || !bytes.Equal(raw, []byte("data: [DONE]\n\n")) {
		t.Fatal("SSE bytes changed")
	}
	if err := restored.Close(); err != nil {
		t.Fatal(err)
	}
	if !sse.closed {
		t.Fatal("inspection lost original body close")
	}
}

func TestProxyBundleCacheIsSharedAndExplicitDirectWins(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"proxies":[{"id":"node","type":"socks5","host":"127.0.0.1","port":19000}]}`)
	store := config.LoadStore()
	c := NewClient(store, nil)
	defer c.Close()
	a := config.Account{Email: "proxy@example.com", ProxyID: "node"}
	expected := c.requestClientsForAccount(a)
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			got := c.requestClientsForAccount(a)
			if got.regular != expected.regular {
				t.Error("concurrent requests got different bundles")
			}
		})
	}
	wg.Wait()
	a.NoProxy = true
	if _, ok := c.resolveProxyForAccount(a); ok {
		t.Fatal("explicit direct ignored")
	}
	direct := c.requestClientsForAccount(a)
	if direct.regular == expected.regular || direct.regular == c.regular {
		t.Fatal("explicit direct reuses proxy/environment bundle")
	}
}

func TestCaptchaStatePromotesStandbyWithoutSolvingChallenge(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"elastic_pool":{"enabled":true,"global_count":1},"accounts":[{"email":"first@example.com"},{"email":"second@example.com"}]}`)
	store := config.LoadStore()
	pool := account.NewPool(store)
	defer pool.Close()
	c := NewClient(store, auth.NewResolver(store, pool, nil))
	defer c.Close()
	a := &auth.RequestAuth{UseConfigToken: true, AccountID: "first@example.com"}
	response := map[string]any{"data": map[string]any{"biz_code": 123, "biz_msg": "captcha required"}}
	if kind := c.observeAccountFailure(a, response, "pow"); kind != FailureCaptchaRequired {
		t.Fatal("captcha not surfaced")
	}
	first, _ := store.FindAccount(a.AccountID)
	second, _ := store.FindAccount("second@example.com")
	if !first.IsCoolingDown() || first.CooldownUntil < float64(time.Now().Unix()) || second.Disabled {
		t.Fatal("captcha did not bench account and promote standby")
	}
}
