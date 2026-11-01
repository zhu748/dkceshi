package usagestats

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

func writeSnapshot(path string, payload fileData, write func(string, []byte) error) error {
	sort.Slice(payload.Entries, func(i, j int) bool {
		a, b := payload.Entries[i], payload.Entries[j]
		if a.Date != b.Date {
			return a.Date < b.Date
		}
		if a.Model != b.Model {
			return a.Model < b.Model
		}
		return entryIdentityLess(a, b)
	})
	body, err := json.MarshalIndent(payload, "", "  ")
	if err == nil {
		err = write(path, append(body, '\n'))
	}
	if err != nil {
		return fmt.Errorf("save usage stats: %w", err)
	}
	return nil
}

// Administrative mutations publish only after an atomic disk write succeeds.
// Both locks are required: flushMu orders writers; mu makes the candidate and
// its commit atomic with respect to live Record calls. Only explicit import,
// clear and initial backfill hold mu over I/O; periodic flushes still do not.
func (s *Store) commitEntriesLocked(entries map[string]*Entry) error {
	payload := fileData{Version: FileVersion, UpdatedAt: time.Now().UnixMilli(), Entries: make([]Entry, 0, len(entries))}
	for _, entry := range entries {
		payload.Entries = append(payload.Entries, *entry)
	}
	if err := writeSnapshot(s.path, payload, writeFileAtomic); err != nil {
		return err
	}
	s.entries = entries
	s.revision++
	s.dirty = false
	return nil
}

// Clear resets the complete ledger only if the reset is durable.
func (s *Store) Clear() error {
	if s == nil {
		return nil
	}
	s.flushMu.Lock()
	defer s.flushMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.commitEntriesLocked(map[string]*Entry{})
}
