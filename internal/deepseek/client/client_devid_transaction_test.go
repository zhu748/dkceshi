package client

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"ds2api/internal/config"
	"ds2api/internal/devid"
)

func newDeviceRefreshClient(t *testing.T, raw string) *Client {
	t.Helper()
	t.Setenv("VERCEL", "")
	t.Setenv("NOW_REGION", "")
	t.Setenv("DS2API_CONFIG_JSON", raw)
	t.Setenv("DS2API_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	t.Setenv("DS2API_ENV_WRITEBACK", "false")
	t.Setenv("DS2API_DEVID_DIR", t.TempDir())
	c := NewClient(config.LoadStore(), nil)
	t.Cleanup(c.Close)
	return c
}

func TestResetFailurePreservesOldProfile(t *testing.T) {
	c := newDeviceRefreshClient(t, `{"runtime":{"device_id_mode":"auto"},"accounts":[{"email":"reset@example.com","password":"p","device_id":"B-old","device_id_type":"auto"}]}`)
	p := devid.ProfilePathFor("reset@example.com")
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(`{"old":"archive"}`), 0600); err != nil {
		t.Fatal(err)
	}
	c.deviceIDGenerator = func(context.Context, devid.Options) (string, error) {
		return "", errors.New("simulated network failure")
	}
	if _, err := c.RefreshRealDeviceID(context.Background(), "reset@example.com", true); err == nil {
		t.Fatal("expected generation failure")
	}
	if _, err := os.Stat(p); err != nil {
		t.Errorf("old device archive lost after failed reset: %v", err)
	}
}

func TestBatchConcurrencyLimitIsGlobal(t *testing.T) {
	c := newDeviceRefreshClient(t, `{"runtime":{"device_id_mode":"auto"},"accounts":[{"email":"a@x","password":"p"},{"email":"b@x","password":"p"},{"email":"c@x","password":"p"},{"email":"d@x","password":"p"}]}`)
	entered, release := make(chan struct{}, 10), make(chan struct{})
	c.deviceIDGenerator = func(ctx context.Context, opts devid.Options) (string, error) {
		entered <- struct{}{}
		select {
		case <-release:
			if err := os.WriteFile(opts.ProfilePath, []byte(`{"version":3,"profile":{}}`), 0600); err != nil {
				return "", err
			}
			return "B-audit", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	var wg sync.WaitGroup
	for _, ids := range [][]string{{"a@x", "b@x"}, {"c@x", "d@x"}} {
		wg.Add(1)
		go func(ids []string) { defer wg.Done(); c.RefreshRealDeviceIDs(context.Background(), ids, false) }(ids)
	}
	count := 0
	deadline := time.After(100 * time.Millisecond)
loop:
	for count < 4 {
		select {
		case <-entered:
			count++
		case <-deadline:
			break loop
		}
	}
	close(release)
	wg.Wait()
	if count > devid.MaxConcurrency {
		t.Errorf("%d simultaneous generators exceed declared concurrency limit %d", count, devid.MaxConcurrency)
	}
}

func TestRefreshRejectsChangedDeviceProfile(t *testing.T) {
	c := newDeviceRefreshClient(t, `{"runtime":{"device_id_mode":"auto"},"accounts":[{"email":"change@example.com","password":"p","device_model":"V2405A","android_api_level":"35"}]}`)
	c.deviceIDGenerator = func(_ context.Context, opts devid.Options) (string, error) {
		if err := c.Store.Update(func(cfg *config.Config) error { cfg.Accounts[0].DeviceModel = "Pixel 9"; return nil }); err != nil {
			return "", err
		}
		if err := os.WriteFile(opts.ProfilePath, []byte(`{"version":3,"profile":{}}`), 0600); err != nil {
			return "", err
		}
		return "B-issued-for-V2405A", nil
	}
	_, err := c.RefreshRealDeviceID(context.Background(), "change@example.com", false)
	acc, _ := c.Store.FindAccount("change@example.com")
	if err == nil && acc.DeviceModel == "Pixel 9" && acc.DeviceID == "B-issued-for-V2405A" {
		t.Error("bound old-model SDK result to new-model account without checking device revision")
	}
}

func TestAndroid12AnchorMapping(t *testing.T) {
	t.Setenv("DS2API_DEVID_DIR", t.TempDir())
	p, err := buildDevidAnchor("api31", config.Account{Email: "api31", DeviceModel: "Pixel 6", AndroidAPILevel: "31"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("API31 anchor: %s", raw)
	var anchor struct {
		AndroidVersion int `json:"android_version"`
	}
	if err := json.Unmarshal(raw, &anchor); err != nil {
		t.Fatal(err)
	}
	if anchor.AndroidVersion != 12 {
		t.Error("Android API 31 must anchor Android 12 rather than fallback 16")
	}
}

func TestDistinctEmailsHaveDistinctArchivePaths(t *testing.T) {
	a, b := devid.ProfilePathFor("user+tag@example.com"), devid.ProfilePathFor("user_tag@example.com")
	if a == b {
		t.Errorf("different valid emails map to the same archive: %s", a)
	}
}

func TestCancelledRefreshStopsWaitingForAccountLock(t *testing.T) {
	c := newDeviceRefreshClient(t, `{"runtime":{"device_id_mode":"auto"},"accounts":[{"email":"lock@example.com","password":"p"}]}`)
	unlock, err := devid.AcquireAccount(context.Background(), "lock@example.com")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	c.deviceIDGenerator = func(ctx context.Context, _ devid.Options) (string, error) { return "", ctx.Err() }
	go func() {
		defer close(done)
		if _, err := c.RefreshRealDeviceID(ctx, "lock@example.com", false); !errors.Is(err, context.Canceled) {
			t.Errorf("cancelled refresh: %v", err)
		}
	}()
	select {
	case <-done:
		unlock()
	case <-time.After(50 * time.Millisecond):
		unlock()
		<-done
		t.Error("cancelled refresh remained blocked on per-account mutex")
	}
}
