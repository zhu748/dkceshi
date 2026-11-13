package auth

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"ds2api/internal/config"
)

func TestJWTCreateVerify(t *testing.T) {
	t.Setenv("DS2API_ADMIN_KEY", "test-admin-key")
	token, err := CreateJWT(1)
	if err != nil {
		t.Fatalf("create jwt failed: %v", err)
	}
	payload, err := VerifyJWT(token)
	if err != nil {
		t.Fatalf("verify jwt failed: %v", err)
	}
	if payload["role"] != "admin" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestVerifyAdminRequest(t *testing.T) {
	t.Setenv("DS2API_ADMIN_KEY", "test-admin-key")
	token, _ := CreateJWT(1)
	req, _ := http.NewRequest(http.MethodGet, "/admin/config", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	if err := VerifyAdminRequest(req); err != nil {
		t.Fatalf("expected token accepted: %v", err)
	}
}

func TestVerifyJWTWithStoreValidAfter(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"admin":{"password_hash":"`+HashAdminPassword("oldpass")+`"}}`)
	store := config.LoadStore()
	token, err := CreateJWTWithStore(1, store)
	if err != nil {
		t.Fatalf("create jwt failed: %v", err)
	}
	if _, err := VerifyJWTWithStore(token, store); err != nil {
		t.Fatalf("verify before invalidation failed: %v", err)
	}
	if err := store.Update(func(c *config.Config) error {
		c.Admin.JWTValidAfterUnix = 1<<62 - 1
		return nil
	}); err != nil {
		t.Fatalf("set valid-after failed: %v", err)
	}
	if _, err := VerifyJWTWithStore(token, store); err == nil {
		t.Fatal("expected token invalid after valid-after update")
	}
}

func TestVerifyJWTWithStoreSameSecondInvalidationAndRelogin(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"admin":{"password_hash":"`+HashAdminPassword("oldpass")+`"}}`)
	store := config.LoadStore()

	oldToken, err := CreateJWTWithStore(1, store)
	if err != nil {
		t.Fatalf("create old jwt failed: %v", err)
	}
	oldPayload, err := VerifyJWTWithStore(oldToken, store)
	if err != nil {
		t.Fatalf("verify old jwt before invalidation failed: %v", err)
	}
	oldIAT, _ := oldPayload["iat"].(float64)

	if err := store.Update(func(c *config.Config) error {
		c.Admin.JWTValidAfterUnix = int64(oldIAT)
		return nil
	}); err != nil {
		t.Fatalf("set valid-after failed: %v", err)
	}

	if _, err := VerifyJWTWithStore(oldToken, store); err == nil {
		t.Fatal("expected old token invalid when iat == valid-after")
	}

	newToken, err := CreateJWTWithStore(1, store)
	if err != nil {
		t.Fatalf("create new jwt failed: %v", err)
	}
	if _, err := VerifyJWTWithStore(newToken, store); err != nil {
		t.Fatalf("expected new token valid after invalidation cutoff: %v", err)
	}
}

func TestJWTRejectsWrongAlgorithmRoleAndExpirationBoundary(t *testing.T) {
	t.Setenv("DS2API_ADMIN_KEY", "jwt-validation-test")
	for _, tc := range []struct {
		name, algorithm, role string
		exp                   int64
	}{
		{"wrong algorithm", "HS512", "admin", time.Now().Add(time.Hour).Unix()},
		{"wrong role", "HS256", "user", time.Now().Add(time.Hour).Unix()},
		{"expires now", "HS256", "admin", time.Now().Unix()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			header, _ := json.Marshal(map[string]any{"alg": tc.algorithm, "typ": "JWT"})
			payload, _ := json.Marshal(map[string]any{"role": tc.role, "exp": tc.exp})
			message := rawB64Encode(header) + "." + rawB64Encode(payload)
			token := message + "." + rawB64Encode(signHS256(message, nil))
			if _, err := VerifyJWT(token); err == nil {
				t.Fatal("accepted invalid JWT claims or header")
			}
		})
	}
}
