package config

import (
	"encoding/base64"
	"path/filepath"
	"sync"
	"testing"
)

func TestAndroidProfilesPersistAndRemainBound(t *testing.T) {
	t.Setenv("VERCEL", "")
	t.Setenv("NOW_REGION", "")
	s := &Store{path: filepath.Join(t.TempDir(), "config.json"), cfg: Config{Accounts: []Account{{Email: "one@example.com", Password: "p"}, {Email: "two@example.com", Password: "p"}}}}
	s.rebuildIndexes()
	first, ok := s.EnsureAccountDeviceProfile("one@example.com")
	if !ok {
		t.Fatal("missing profile")
	}
	raw, err := base64.StdEncoding.DecodeString(first.DeviceID)
	if err != nil || len(raw) != 32 || first.DeviceModel == "" || first.AndroidAPILevel == "" {
		t.Fatal("incomplete Android identity")
	}
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			got, _ := s.EnsureAccountDeviceProfile(first.Identifier())
			if got.DeviceID != first.DeviceID || got.DeviceModel != first.DeviceModel {
				t.Error("concurrent requests changed identity")
			}
		})
	}
	wg.Wait()
	other, _ := s.EnsureAccountDeviceProfile("two@example.com")
	if other.DeviceID == first.DeviceID {
		t.Fatal("accounts share ID")
	}
	if err := s.Update(func(c *Config) error { c.Accounts[0].Disabled = true; return nil }); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DS2API_CONFIG_JSON", "")
	t.Setenv("DS2API_CONFIG_PATH", s.path)
	reloaded, err := LoadStoreWithError()
	if err != nil {
		t.Fatal(err)
	}
	got, _ := reloaded.EnsureAccountDeviceProfile(first.Identifier())
	if got.DeviceID != first.DeviceID || got.DeviceModel != first.DeviceModel || got.AndroidAPILevel != first.AndroidAPILevel {
		t.Fatal("profile lost on save, sleep or restart")
	}
}

func TestAndroidProfilesRespectOverridesAndReadOnlyRestarts(t *testing.T) {
	t.Setenv("DS2API_DEEPSEEK_DEVICE_MODEL", "Custom Phone")
	t.Setenv("DS2API_DEEPSEEK_ANDROID_API_LEVEL", "35")
	t.Setenv("DS2API_DEEPSEEK_DEVICE_ID", "manual-env-id")
	got := fillAndroidProfile(Account{Email: "one@example.com"}, true)
	if got.DeviceModel != "Custom Phone" || got.AndroidAPILevel != "35" || got.DeviceID != "manual-env-id" {
		t.Fatal("environment override lost")
	}
	manual := Account{Email: "one@example.com", DeviceModel: "Pixel 9", AndroidAPILevel: "36", DeviceID: "account-id"}
	if got := fillAndroidProfile(manual, false); got != manual {
		t.Fatal("account override lost")
	}
	t.Setenv("DS2API_DEEPSEEK_DEVICE_MODEL", "")
	t.Setenv("DS2API_DEEPSEEK_ANDROID_API_LEVEL", "")
	t.Setenv("DS2API_DEEPSEEK_DEVICE_ID", "")
	if a, b := fillAndroidProfile(Account{Email: "one@example.com"}, true), fillAndroidProfile(Account{Email: "one@example.com"}, true); a != b {
		t.Fatal("read-only restart changed identity")
	}
}
