package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDeviceSnapshotRejectsDeleteAndIdenticalReAdd(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"accounts":[{"email":"same@example.com","password":"p"}]}`)
	t.Setenv("DS2API_ENV_WRITEBACK", "false")
	s := LoadStore()
	snapshot, _ := s.DeviceSnapshot("same@example.com")
	if err := s.Update(func(c *Config) error { c.Accounts = nil; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(func(c *Config) error { c.Accounts = []Account{snapshot.Account}; return nil }); err != nil {
		t.Fatal(err)
	}
	called := false
	err := s.CommitGeneratedDeviceID(context.Background(), snapshot, "B-old-generation", func() (func() error, error) { called = true; return func() error { return nil }, nil })
	if err == nil || called {
		t.Fatal("accepted generation for a deleted account incarnation")
	}
}

func TestDeviceIDCommitRollsBackMemoryAndArchiveOnConfigWriteFailure(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DS2API_CONFIG_JSON", `{"runtime":{"device_id_mode":"auto"},"accounts":[{"email":"rollback@example.com","password":"p","device_id":"B-old","device_id_type":"auto"}]}`)
	t.Setenv("DS2API_CONFIG_PATH", filepath.Join(blocker, "config.json"))
	t.Setenv("DS2API_ENV_WRITEBACK", "false")
	s := LoadStore()
	snapshot, _ := s.DeviceSnapshot("rollback@example.com")
	t.Setenv("DS2API_ENV_WRITEBACK", "true")
	promoted, rolledBack := false, false
	err := s.CommitGeneratedDeviceID(context.Background(), snapshot, "B-new", func() (func() error, error) {
		promoted = true
		return func() error { rolledBack = true; return nil }, nil
	})
	acc, _ := s.FindAccount("rollback@example.com")
	if err == nil || !promoted || !rolledBack || acc.DeviceID != "B-old" {
		t.Fatalf("failed rollback: %v promoted=%v rollback=%v account=%+v", err, promoted, rolledBack, acc)
	}
}

func TestDeviceProfileChangesInvalidateOnlyUnchangedAutoIDs(t *testing.T) {
	base := Config{Accounts: []Account{{Email: "a@x", DeviceModel: "old", DeviceID: "B-old", DeviceIDType: DeviceIDTypeAuto}}}
	for _, sample := range []struct{ id, want string }{{"B-old", ""}, {"B-explicit-new", "B-explicit-new"}} {
		next := base.Clone()
		next.Accounts[0].DeviceModel = "new"
		next.Accounts[0].DeviceID = sample.id
		InvalidateChangedDeviceIDs(base, &next)
		if next.Accounts[0].DeviceID != sample.want {
			t.Fatalf("replacement ID lost: %+v", next.Accounts[0])
		}
	}
}

func TestDeviceSnapshotRejectsChangedProxyDefinition(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"accounts":[{"email":"proxy@example.com","password":"p","proxy_id":"p"}],"proxies":[{"id":"p","type":"socks5h","host":"old.invalid","port":1080}]}`)
	t.Setenv("DS2API_ENV_WRITEBACK", "false")
	s := LoadStore()
	snapshot, _ := s.DeviceSnapshot("proxy@example.com")
	if err := s.Update(func(c *Config) error { c.Proxies[0].Host = "new.invalid"; return nil }); err != nil {
		t.Fatal(err)
	}
	promoted := false
	err := s.CommitGeneratedDeviceID(context.Background(), snapshot, "B-old-route", func() (func() error, error) { promoted = true; return func() error { return nil }, nil })
	if err == nil || promoted {
		t.Fatal("a changed proxy definition accepted the old report")
	}
}
