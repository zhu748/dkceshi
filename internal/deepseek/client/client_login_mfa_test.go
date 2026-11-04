package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/config"
)

func TestCapturedMFAResponseIsNotRejectedCredentialsOrLoginSuccess(t *testing.T) {
	raw, err := os.ReadFile("../../../tests/fixtures/android_app_2_7_1_login_2026_11_04.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Response json.RawMessage `json:"login_response"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Response) == 0 {
		t.Fatal("missing sanitized MFA response")
	}
	t.Setenv("DS2API_CONFIG_JSON", `{"accounts":[{"email":"mfa@example.com","password":"p","device_model":"Pixel 9a","android_api_level":"36","device_id":"sdk-risk-id","installation_id":"local-installation"}]}`)
	store := config.LoadStore()
	c := NewClient(store, nil)
	defer c.Close()
	calls := 0
	c.regular = alignmentDoer(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/api/v0/users/login" {
			t.Fatal("MFA response must not start authenticated requests")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(fixture.Response))}, nil
	})
	token, err := c.Login(context.Background(), config.Account{Email: "mfa@example.com", Password: "p"})
	var failure *RequestFailure
	if token != "" || !errors.As(err, &failure) || failure.Kind != FailureCaptchaRequired || errors.Is(err, auth.ErrLoginRejected) {
		t.Fatalf("unexpected MFA result: token present=%v error=%v", token != "", err)
	}
	if calls != 1 || strings.Contains(err.Error(), "synthetic-mfa-session") {
		t.Fatal("MFA session leaked or login continued without verification")
	}
	account, _ := store.FindAccount("mfa@example.com")
	if account.AuthFailed || account.Disabled || account.Token != "" {
		t.Fatal("MFA prompt changed account availability or token")
	}
}
