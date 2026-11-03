package usagestats

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestSummaryRevisionDescribesTheSameSnapshot(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "usage.json"))
	defer s.Close()
	done := make(chan struct{})
	at := time.Now()
	go func() {
		defer close(done)
		for range 500 {
			s.Record("model", "caller", map[string]any{"prompt_tokens": 10}, at)
		}
	}()
	for range 500 {
		entries, revision := s.SummaryWithRevision()
		var calls int64
		if len(entries) > 0 {
			calls = entries[0].Calls
			entries[0].Calls = -1 // Snapshot changes must not affect the store.
		}
		if revision != fmt.Sprintf("%s-%d", s.instanceID, calls) {
			t.Errorf("revision %q does not describe %d recorded calls", revision, calls)
			break
		}
	}
	<-done
	if got := s.Summary(); len(got) != 1 || got[0].Calls != 500 {
		t.Fatalf("snapshot mutation leaked into live entries: %+v", got)
	}
}

func TestCompactCheckpointRetainsSchemaAndFailedFlushCanRetry(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "usage.json"))
	defer s.Close()
	s.Record("model", "caller", map[string]any{"prompt_tokens": 10}, time.Now())
	failed := errors.New("disk failure")
	if err := s.flushWith(func(string, []byte) error { return failed }); !errors.Is(err, failed) {
		t.Fatalf("flush error was lost: %v", err)
	}
	var raw []byte
	if err := s.flushWith(func(_ string, body []byte) error { raw = append([]byte(nil), body...); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 || bytes.Contains(raw, []byte("\n ")) {
		t.Fatal("retry did not write a compact checkpoint")
	}
	var payload fileData
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Version != FileVersion || len(payload.Entries) != 1 || payload.Entries[0].TotalTokens != 10 {
		t.Fatalf("checkpoint changed ledger values: %+v", payload)
	}
	if err := s.flushWith(func(string, []byte) error { t.Error("clean ledger was rewritten"); return nil }); err != nil {
		t.Fatal(err)
	}
}
