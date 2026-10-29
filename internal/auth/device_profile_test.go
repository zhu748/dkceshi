package auth

import (
	"context"
	"testing"

	"ds2api/internal/config"
)

func TestEnsureTokenAddsProfileWithoutReplacingCredentials(t *testing.T) {
	r := newTestResolver(t)
	r.Login = nil
	a := r.NewKeepAliveAuth(config.Account{Email: "acc@example.com", Token: "already-held-token", Password: "request-password"})
	if err := r.EnsureManagedToken(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	if a.DeepSeekToken != "already-held-token" || a.Account.Password != "request-password" {
		t.Fatal("profile hydration replaced request credentials")
	}
	if a.Account.DeviceModel == "" || a.Account.InstallationID == "" {
		t.Fatal("profile hydration missing")
	}
}
