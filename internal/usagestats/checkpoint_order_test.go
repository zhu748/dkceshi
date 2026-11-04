package usagestats

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func BenchmarkFlushStableLedger(b *testing.B) {
	for _, n := range []int{10000, 100000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			s := &Store{entries: make(map[string]*Entry, n), path: "unused", dirty: true}
			for _, entry := range benchmarkEntries(n) {
				copy := entry
				s.entries[entryKey(entry.Date, entry.Model, entry.CallerID, entry.AccountID)] = &copy
			}
			write := func(string, []byte) error { return nil }
			if err := s.flushWith(write); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				s.dirty = true
				s.revision++
				if err := s.flushWith(write); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestCheckpointOrderInvalidatesForNewKeysAndReplacement(t *testing.T) {
	s := &Store{entries: map[string]*Entry{}, settings: DefaultSettings(), path: filepath.Join(t.TempDir(), "usage.json")}
	var saved fileData
	write := func(_ string, body []byte) error { return json.Unmarshal(body, &saved) }
	at := time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)
	record := func(model string) { s.Record(model, "caller", map[string]any{"prompt_tokens": 10}, at) }
	record("z")
	record("a")
	if err := s.flushWith(write); err != nil {
		t.Fatal(err)
	}
	if len(s.checkpointOrder) != 2 || saved.Entries[0].Model != "a" {
		t.Fatal("initial order is wrong")
	}
	record("z")
	record("middle")
	if err := s.flushWith(write); err != nil {
		t.Fatal(err)
	}
	if len(saved.Entries) != 3 || saved.Entries[1].Model != "middle" || saved.Entries[2].Calls != 2 {
		t.Fatalf("stale cached keys: %+v", saved)
	}
	if err := s.Clear(); err != nil {
		t.Fatal(err)
	}
	record("replacement")
	if err := s.flushWith(write); err != nil {
		t.Fatal(err)
	}
	if len(saved.Entries) != 1 || saved.Entries[0].Model != "replacement" {
		t.Fatal("clear retained stale keys")
	}
	if _, err := s.Import([]Entry{{Date: "2026-11-04", Model: "imported", PromptTokens: 5, TotalTokens: 5, Calls: 1}}); err != nil {
		t.Fatal(err)
	}
	record("replacement")
	if err := s.flushWith(write); err != nil {
		t.Fatal(err)
	}
	if len(saved.Entries) != 2 || saved.Entries[0].Model != "imported" {
		t.Fatalf("import retained stale order: %+v", saved)
	}
}
func TestCheckpointOrderSurvivesFailedWriteAndConcurrentNewKey(t *testing.T) {
	s := &Store{entries: map[string]*Entry{}, settings: DefaultSettings(), path: "unused"}
	at := time.Now()
	record := func(model string) { s.Record(model, "caller", map[string]any{"prompt_tokens": 10}, at) }
	record("first")
	err := s.flushWith(func(string, []byte) error { record("new"); return errors.New("disk unavailable") })
	if err == nil || !s.dirty {
		t.Fatal("failed write committed")
	}
	var saved fileData
	if err := s.flushWith(func(_ string, body []byte) error { return json.Unmarshal(body, &saved) }); err != nil {
		t.Fatal(err)
	}
	if len(saved.Entries) != 2 || s.dirty {
		t.Fatalf("concurrent entry was lost: %+v", saved)
	}
	record("first")
	if err := s.flushWith(func(string, []byte) error { return errors.New("disk unavailable") }); err == nil {
		t.Fatal("expected failure")
	}
	if err := s.flushWith(func(_ string, body []byte) error { return json.Unmarshal(body, &saved) }); err != nil {
		t.Fatal(err)
	}
	if saved.Entries[0].Calls != 2 {
		t.Fatal("failed flush lost a counter")
	}
}
