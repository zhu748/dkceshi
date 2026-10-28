package client

import (
	"context"
	"encoding/base64"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/config"
)

func TestAccountDeviceIdentityIsConsistentAcrossRequests(t *testing.T) {
	a := &auth.RequestAuth{Account: config.Account{Email: "one@example.com"}, DeepSeekToken: "first-token"}
	first := BaseHeadersForAuth(a)
	if first["x-device-id"] != loginDeviceID(a.Account) {
		t.Fatal("login and authenticated requests use different device IDs")
	}
	decoded, err := base64.StdEncoding.DecodeString(first["x-device-id"])
	if err != nil || len(decoded) != 32 {
		t.Fatalf("device ID format mismatch")
	}
	a.DeepSeekToken = "refreshed-token"
	if BaseHeadersForAuth(a)["x-device-id"] != first["x-device-id"] {
		t.Fatal("refresh changed installation identity")
	}
	b := &auth.RequestAuth{Account: config.Account{Email: "two@example.com"}}
	if BaseHeadersForAuth(b)["x-device-id"] == first["x-device-id"] || BaseHeadersForAuth(b)["x-rangers-id"] == first["x-rangers-id"] {
		t.Fatal("accounts share a fallback installation identity")
	}
	a.Account.DeviceID = "configured-device"
	a.Account.DeviceModel = "Configured Phone"
	a.Account.RangersID = "configured-rangers"
	got := BaseHeadersForAuth(a)
	if got["x-device-id"] != "configured-device" || got["x-device-model"] != "Configured Phone" || got["x-rangers-id"] != "configured-rangers" || loginDeviceID(a.Account) != "configured-device" {
		t.Fatal("configured identity lost")
	}
	c := &Client{}
	ctx := auth.WithAuth(context.Background(), a)
	if c.authHeadersFromContext(ctx, a.DeepSeekToken)["x-device-id"] != "configured-device" {
		t.Fatal("cleanup lost account identity")
	}
	got["x-device-id"] = "mutated"
	if BaseHeadersForAuth(a)["x-device-id"] != "configured-device" {
		t.Fatal("headers were shared mutable state")
	}
}
