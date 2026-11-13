package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRegressionFailedWriteMustNotPublish(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	t.Setenv("DS2API_CONFIG_PATH", path)
	t.Setenv("DS2API_CONFIG_JSON", "")
	t.Setenv("VERCEL", "")
	t.Setenv("NOW_REGION", "")
	if err := os.WriteFile(path, []byte(`{"keys":["old-key"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := LoadStoreWithError()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	err = s.Update(func(c *Config) error { c.Keys = []string{"new-key"}; return nil })
	if err == nil {
		t.Fatal("expected persistence failure")
	}
	if s.HasAPIKey("new-key") || !s.HasAPIKey("old-key") {
		t.Fatalf("failed write published new credential: error=%v, old=%v new=%v", err, s.HasAPIKey("old-key"), s.HasAPIKey("new-key"))
	}
}

func TestFailedReplaceRestoresSubscriptionFileAndMemory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	subs := filepath.Join(dir, "subscriptions.json")
	t.Setenv("DS2API_CONFIG_PATH", path)
	t.Setenv("DS2API_CONFIG_JSON", "")
	t.Setenv("DS2API_MIHOMO_SUBSCRIPTIONS_PATH", subs)
	t.Setenv("VERCEL", "")
	t.Setenv("NOW_REGION", "")
	original := []byte(`{"version":1,"port_map":{"original":12345}}`)
	if err := os.WriteFile(path, []byte(`{"keys":["old"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(subs, original, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := LoadStoreWithError()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	candidate := s.Snapshot()
	candidate.Keys = []string{"new"}
	candidate.APIKeys = nil
	candidate.Mihomo.PortMap = map[string]int{"replacement": 12346}
	if err := s.Replace(candidate); err == nil {
		t.Fatal("expected failed main config promotion")
	}
	if !s.HasAPIKey("old") || s.HasAPIKey("new") {
		t.Fatal("failed replacement changed credentials")
	}
	got, err := os.ReadFile(subs)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatalf("subscription rollback failed: %s", got)
	}
}
