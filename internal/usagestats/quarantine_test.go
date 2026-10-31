package usagestats

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Regression: an unreadable (corrupt JSON) ledger used to be silently replaced
// by a near-empty one — the first Record flushed and atomically overwrote the
// original file, losing the whole history with no backup. The store must
// quarantine the unreadable file aside first.
func TestCorruptLedgerIsQuarantinedNotOverwritten(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "usage.json")
	corrupt := []byte("{definitely not json")
	if err := os.WriteFile(path, corrupt, 0o644); err != nil {
		t.Fatal(err)
	}

	store := New(path)
	defer store.Close()
	store.Record("model", "caller", map[string]any{"total_tokens": 5}, time.Now())
	if err := store.flushNow(); err != nil {
		t.Fatalf("flush failed: %v", err)
	}

	// The new ledger must be valid JSON at the original path.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read new ledger failed: %v", err)
	}
	var persisted fileData
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatalf("new ledger is not valid JSON: %v", err)
	}

	// The original corrupt bytes must be preserved in a .corrupt-* sibling.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	quarantined := false
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, "usage.json.corrupt-") {
			continue
		}
		saved, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read quarantined ledger failed: %v", err)
		}
		if string(saved) != string(corrupt) {
			t.Fatalf("quarantined ledger content changed: %q", saved)
		}
		quarantined = true
	}
	if !quarantined {
		t.Fatalf("expected a quarantined .corrupt-* file next to the ledger, dir=%v", entries)
	}
}
