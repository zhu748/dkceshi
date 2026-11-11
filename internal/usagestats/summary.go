package usagestats

import (
	"slices"
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
	// A flush already orders these keys by date/model/identity. Recording
	// changes values without invalidating that order; a new key changes its
	// length, and import/clear invalidate it under the writer lock.
	ordered := len(s.checkpointOrder) == len(s.entries)
	if ordered {
		for _, key := range s.checkpointOrder {
			out = append(out, *s.entries[key])
		}
	} else {
		for _, entry := range s.entries {
			out = append(out, *entry)
		}
	}
	s.mu.Unlock()
	if ordered {
		// Checkpoints sort dates ascending; the UI wants dates descending
		// while models and identities within each date remain ascending.
		slices.Reverse(out)
		for start := 0; start < len(out); {
			end := start + 1
			for end < len(out) && out[end].Date == out[start].Date {
				end++
			}
			slices.Reverse(out[start:end])
			start = end
		}
		return out, version
	}
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
