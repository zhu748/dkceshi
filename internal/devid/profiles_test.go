package devid

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"ds2api/internal/config"
)

func writeTestProfile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestProfileMigrationAndAmbiguousLegacyIsolation(t *testing.T) {
	t.Setenv("DS2API_DEVID_DIR", t.TempDir())
	const old = `{"version":3,"profile":{"seed":123},"smid_v2":"same"}`
	writeTestProfile(t, LegacyProfilePathFor("unique@example.com"), old)
	update, err := PrepareProfile("unique@example.com", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	defer update.Close()
	body, err := os.ReadFile(update.Path)
	if err != nil || string(body) != old {
		t.Fatalf("legacy archive not seeded: %s %v", body, err)
	}
	rollback, err := update.Promote()
	if err != nil {
		t.Fatal(err)
	}
	if err := rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ProfilePathFor("unique@example.com")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("rollback retained new archive")
	}
	if _, err := os.Stat(LegacyProfilePathFor("unique@example.com")); err != nil {
		t.Fatal("failed migration removed original")
	}
	writeTestProfile(t, LegacyProfilePathFor("user+tag@example.com"), old)
	ambiguous, err := PrepareProfile("user+tag@example.com", []config.Account{{Email: "user_tag@example.com"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ambiguous.Close()
	if _, err := os.Stat(ambiguous.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("ambiguous legacy archive was reused")
	}
	if err := RemoveProfiles("user+tag@example.com", []config.Account{{Email: "user_tag@example.com"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(LegacyProfilePathFor("user_tag@example.com")); err != nil {
		t.Fatal("another account's ambiguous archive was removed")
	}
}

func TestProfilePromotionRestoresPreviousArchiveOnRollback(t *testing.T) {
	t.Setenv("DS2API_DEVID_DIR", t.TempDir())
	path := ProfilePathFor("rollback@example.com")
	writeTestProfile(t, path, `{"version":3,"profile":{"old":true}}`)
	update, err := PrepareProfile("rollback@example.com", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	defer update.Close()
	writeTestProfile(t, update.Path, `{"version":3,"profile":{"new":true}}`)
	rollback, err := update.Promote()
	if err != nil {
		t.Fatal(err)
	}
	if err := rollback(); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil || string(body) != `{"version":3,"profile":{"old":true}}` {
		t.Fatalf("rollback failed: %s %v", body, err)
	}
}

func TestAccountLockCancellationDoesNotSplitWaiters(t *testing.T) {
	unlock, err := AcquireAccount(context.Background(), "same")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := AcquireAccount(ctx, "same"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled waiter: %v", err)
	}
	unlock()
	unlock, err = AcquireAccount(context.Background(), "same")
	if err != nil {
		t.Fatal(err)
	}
	unlock()
	accountLocks.Lock()
	defer accountLocks.Unlock()
	if len(accountLocks.items) != 0 {
		t.Fatal("lock entries leaked")
	}
}
