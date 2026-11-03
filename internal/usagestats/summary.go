package usagestats

import (
	"sort"
	"strconv"
)

// Revision identifies a ledger state within this store instance. A new
// instance has a new identity, even if its in-memory counter starts at zero.
func (s *Store) Revision() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.revisionLocked()
}

func (s *Store) revisionLocked() string {
	return s.instanceID + "-" + strconv.FormatInt(s.revision, 10)
}

// Summary returns entries ordered by descending date and ascending model,
// caller and account. Recorded costs are never recalculated on reads.
func (s *Store) Summary() []Entry {
	entries, _ := s.SummaryWithRevision()
	return entries
}

// SummaryWithRevision captures values and revision under the same lock; sorting
// happens on the independent snapshot after releasing the recording lock.
func (s *Store) SummaryWithRevision() ([]Entry, string) {
	if s == nil {
		return []Entry{}, ""
	}
	s.mu.Lock()
	version := s.revisionLocked()
	out := make([]Entry, 0, len(s.entries))
	for _, entry := range s.entries {
		out = append(out, *entry)
	}
	s.mu.Unlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date > out[j].Date
		}
		if out[i].Model != out[j].Model {
			return out[i].Model < out[j].Model
		}
		return entryIdentityLess(out[i], out[j])
	})
	return out, version
}
