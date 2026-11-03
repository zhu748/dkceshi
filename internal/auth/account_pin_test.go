package auth

import (
	"context"
	"testing"
)

func TestTemporaryAccountPinsNestAndPreserveManualBinding(t *testing.T) {
	a := &RequestAuth{AccountID: "owner"}
	outer, inner := a.PinAccount(), a.PinAccount()
	outer()
	outer()
	if !a.AccountPinned() || a.TargetAccount != "" {
		t.Fatal("nested pin lost or manual target modified")
	}
	inner()
	if a.AccountPinned() {
		t.Fatal("temporary pin leaked")
	}
	a.TargetAccount = "manual"
	release := a.PinAccount()
	release()
	if !a.AccountPinned() || a.TargetAccount != "manual" {
		t.Fatal("manual target lost")
	}
	if (*RequestAuth)(nil).AccountPinned() {
		t.Fatal("nil auth pinned")
	}
}

func TestRepeatedPinnedAuthRecoveryCountsRejectedRefreshOnce(t *testing.T) {
	r, p := newSwitchWaitResolver(t)
	defer p.Close()
	a := determineManaged(t, r)
	defer r.Release(a)
	if !a.RefreshTokenOnce(context.Background()) {
		t.Fatal("refresh failed")
	}
	release := a.PinAccount()
	for i := 0; i < 2; i++ {
		if a.RecoverAuthFailure(context.Background()) != AuthRecoveryNone {
			t.Fatal("switched pinned account")
		}
	}
	release()
	acc, _ := r.Store.FindAccount(a.AccountID)
	if acc.AuthFailed {
		t.Fatal("one request counted twice as consecutive failures")
	}
	if a.RecoverAuthFailure(context.Background()) != AuthRecoverySwitched {
		t.Fatal("outer recovery did not switch after unpin")
	}
}
