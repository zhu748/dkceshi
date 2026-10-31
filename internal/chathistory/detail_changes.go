package chathistory

import (
	"sort"
	"strings"
)

func (s *Store) markDetailDirtyLocked(id string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	if s.dirty == nil {
		s.dirty = map[string]struct{}{}
	}
	if s.deleted == nil {
		s.deleted = map[string]struct{}{}
	}
	s.dirty[id] = struct{}{}
	delete(s.deleted, id)
}

func (s *Store) markDetailDeletedLocked(id string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	if s.dirty == nil {
		s.dirty = map[string]struct{}{}
	}
	if s.deleted == nil {
		s.deleted = map[string]struct{}{}
	}
	s.deleted[id] = struct{}{}
	delete(s.dirty, id)
}

func (s *Store) clearPendingDetailChangesLocked() {
	s.dirty = map[string]struct{}{}
	s.deleted = map[string]struct{}{}
}

func sortedDetailIDs(ids map[string]struct{}) []string {
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
