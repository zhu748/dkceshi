package chathistory

import (
	"path/filepath"
	"testing"
)

// Regression: markInterruptedStreamingLocked used to rewrite leftover "streaming"
// entries without bumping the revision. The admin UI polls list/detail with
// If-None-Match, so after a restart the ETag kept matching the pre-crash
// revision and open pages displayed the spinner forever.
func TestRestartInterruptedStreamingBumpsRevision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chat_history.json")
	store := New(path)

	started, err := store.Start(StartParams{
		CallerID:  "caller:abc",
		Model:     "deepseek-v4.1-flash",
		Stream:    true,
		UserInput: "hello",
	})
	if err != nil {
		t.Fatalf("start entry failed: %v", err)
	}
	detailBefore, err := store.DetailRevision(started.ID)
	if err != nil {
		t.Fatalf("detail revision before restart failed: %v", err)
	}
	globalBefore, err := store.Revision()
	if err != nil {
		t.Fatalf("global revision before restart failed: %v", err)
	}

	reloaded := New(path)
	detailAfter, err := reloaded.DetailRevision(started.ID)
	if err != nil {
		t.Fatalf("detail revision after restart failed: %v", err)
	}
	globalAfter, err := reloaded.Revision()
	if err != nil {
		t.Fatalf("global revision after restart failed: %v", err)
	}
	if detailAfter <= detailBefore {
		t.Fatalf("expected detail revision to bump after interrupted rewrite: before=%d after=%d", detailBefore, detailAfter)
	}
	if globalAfter <= globalBefore {
		t.Fatalf("expected global revision to bump after interrupted rewrite: before=%d after=%d", globalBefore, globalAfter)
	}
	full, err := reloaded.Get(started.ID)
	if err != nil {
		t.Fatalf("get detail failed: %v", err)
	}
	if full.Status != "error" {
		t.Fatalf("expected interrupted entry status=error, got %q", full.Status)
	}
	if full.StatusCode != 503 {
		t.Fatalf("expected interrupted entry status code 503, got %d", full.StatusCode)
	}
}
