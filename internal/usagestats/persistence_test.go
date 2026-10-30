package usagestats

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestConcurrentFlushPreservesNewestLedger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.json")
	store := New(path)
	defer store.Close()
	usage := map[string]any{"prompt_tokens": 10}
	at := time.Date(2026, 10, 31, 12, 0, 0, 0, gmt8)
	store.Record("model", "caller", usage, at)

	entered, release := make(chan struct{}), make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- store.flushWith(func(path string, body []byte) error {
			close(entered)
			<-release
			return writeFileAtomic(path, body)
		})
	}()
	<-entered
	store.Record("model", "caller", usage, at)
	secondDone := make(chan error, 1)
	go func() { secondDone <- store.flushNow() }()

	// Let a competing flush finish first if serialization is missing. A slow
	// older disk write must never overwrite an already committed newer ledger.
	var secondErr error
	secondFinished := false
	select {
	case secondErr = <-secondDone:
		secondFinished = true
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if !secondFinished {
		secondErr = <-secondDone
	}
	if secondErr != nil {
		t.Fatal(secondErr)
	}
	// Close must not hide the bug by rewriting a spuriously clean ledger.
	store.Close()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var persisted fileData
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatal(err)
	}
	if len(persisted.Entries) != 1 || persisted.Entries[0].Calls != 2 || persisted.Entries[0].TotalTokens != 20 {
		t.Fatalf("stale flush replaced the latest ledger: %+v", persisted.Entries)
	}
}

func TestSaveSettingsOwnsInputPointers(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "usage.json"))
	defer store.Close()
	settings := DefaultSettings()
	if err := store.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	before := store.Settings()
	*settings.Models[mergedModelPriceKey].InputMissPrice = 999
	*settings.CacheHit.Enabled = false
	*settings.CacheHit.Rate = 0
	if got := store.Settings(); !reflect.DeepEqual(got, before) {
		t.Fatalf("caller mutated saved settings: got %+v, want %+v", got, before)
	}
}

func TestFailedSettingsSaveRetainsPreviousValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.json")
	store := New(path)
	defer store.Close()
	before := store.Settings()
	// An existing directory cannot be replaced with a settings file.
	if err := os.Mkdir(path+".settings", 0o755); err != nil {
		t.Fatal(err)
	}
	settings := store.Settings()
	settings.Enabled = false
	if err := store.SaveSettings(settings); err == nil {
		t.Fatal("expected settings persistence failure")
	}
	if got := store.Settings(); !reflect.DeepEqual(got, before) {
		t.Fatal("failed save changed live settings")
	}
}
