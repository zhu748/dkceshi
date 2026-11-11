package usagestats

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func BenchmarkSummaryStableLedger(b *testing.B) {
	for _, n := range []int{10000, 100000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			s := &Store{entries: make(map[string]*Entry, n), dirty: true}
			for _, entry := range benchmarkEntries(n) {
				copy := entry
				s.entries[entryKey(entry.Date, entry.Model, entry.CallerID, entry.AccountID)] = &copy
			}
			if err := s.flushWith(func(string, []byte) error { return nil }); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			for b.Loop() {
				entries, _ := s.SummaryWithRevision()
				if len(entries) != n {
					b.Fatal(len(entries))
				}
			}
		})
	}
}

func TestSummaryOrderStaysFreshAcrossCheckpointAndMutations(t *testing.T) {
	s := &Store{entries: map[string]*Entry{}, settings: DefaultSettings(), path: filepath.Join(t.TempDir(), "usage.json")}
	at := time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)
	for _, e := range []Entry{
		{Date: "2026-11-03", Model: "z", CallerID: "c", AccountID: "a", TotalTokens: 1, Calls: 1},
		{Date: "2026-11-04", Model: "z", CallerID: "c", AccountID: "a", TotalTokens: 1, Calls: 1},
		{Date: "2026-11-04", Model: "a", CallerID: "c", AccountID: "z", TotalTokens: 1, Calls: 1},
		{Date: "2026-11-04", Model: "a", CallerID: "c", AccountID: "a", TotalTokens: 1, Calls: 1},
	} {
		copy := e
		s.entries[entryKey(e.Date, e.Model, e.CallerID, e.AccountID)] = &copy
	}
	check := func() {
		t.Helper()
		previous, previousVersion := s.SummaryWithRevision()
		// Capture an independently sorted read before a checkpoint exists,
		// then compare the optimized read of exactly the same ledger state.
		s.checkpointOrder = nil
		want, version := s.SummaryWithRevision()
		if !reflect.DeepEqual(previous, want) || previousVersion != version {
			t.Fatalf("live summary used stale ordering: got %+v (%s), want %+v (%s)", previous, previousVersion, want, version)
		}
		s.dirty = true
		if err := s.flushWith(func(string, []byte) error { return nil }); err != nil {
			t.Fatal(err)
		}
		got, gotVersion := s.SummaryWithRevision()
		if !reflect.DeepEqual(got, want) || gotVersion != version {
			t.Fatalf("checkpoint changed summary or revision: got %+v (%s), want %+v (%s)", got, gotVersion, want, version)
		}
		if len(got) > 0 {
			got[0].TotalTokens = -1
			if reflect.DeepEqual(s.Summary(), got) {
				t.Fatal("summary exposed mutable store data")
			}
		}
	}
	check()
	s.RecordAccount("a", "c", "a", "latest label", map[string]any{"prompt_tokens": 10}, at)
	if got := s.Summary(); got[0].TotalTokens != 11 || got[0].AccountLabel != "latest label" {
		t.Fatalf("checkpoint hid live counters: %+v", got)
	}
	check()
	s.Record("middle", "caller", map[string]any{"prompt_tokens": 10}, at)
	check()
	if err := s.Clear(); err != nil {
		t.Fatal(err)
	}
	s.Record("replacement", "caller", map[string]any{"prompt_tokens": 5}, at)
	check()
	// Replace the entire map while preserving its size: length alone must
	// not allow keys from a previous checkpoint to survive an admin import.
	s.flushMu.Lock()
	s.mu.Lock()
	err := s.commitEntriesLocked(map[string]*Entry{"imported": {Date: "2026-11-04", Model: "imported", CallerID: "caller", TotalTokens: 2, Calls: 1}})
	s.mu.Unlock()
	s.flushMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Summary(); len(got) != 1 || got[0].Model != "imported" {
		t.Fatalf("stale replacement: %+v", got)
	}
	check()
}
