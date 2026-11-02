package tooldiagnostics

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var ErrNotFound = errors.New("tool diagnostic record not found")

// Only summaries reside in the store; retained failures' bodies are loaded on
// demand. mu serializes disk commits as well as retention decisions.
type Store struct {
	mu        sync.Mutex
	path      string
	state     Index
	lastError error
	loadError error
}

func (s *Store) Enabled() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadError == nil && s.state.Settings.Enabled
}

func (s *Store) Put(record Record) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadError != nil {
		return s.loadError
	}
	if !validID(record.ID) {
		return errors.New("invalid diagnostic id")
	}
	if err := atomicJSON(s.detailPath(record.ID), record); err != nil {
		s.lastError = err
		return err
	}
	before := s.state
	s.state.Items = append([]Summary(nil), s.state.Items...)
	found := false
	reorder := false
	for i := range s.state.Items {
		if s.state.Items[i].ID == record.ID {
			reorder = s.state.Items[i].CreatedAt != record.CreatedAt
			s.state.Items[i] = record.Summary
			found = true
			break
		}
	}
	if !found {
		s.state.Items = append(s.state.Items, record.Summary)
		reorder = true
	}
	if reorder {
		sort.Slice(s.state.Items, func(i, j int) bool {
			a, b := s.state.Items[i], s.state.Items[j]
			if a.CreatedAt == b.CreatedAt {
				return a.ID > b.ID
			}
			return a.CreatedAt > b.CreatedAt
		})
	}
	if err := s.commitLocked(s.trimLocked()); err != nil {
		s.state = before
		return err
	}
	return nil
}

func (s *Store) trimLocked() []string {
	kept := make([]Summary, 0, len(s.state.Items))
	removed := []string{}
	ordinary := 0
	for _, item := range s.state.Items {
		if !item.Pinned && item.CompletedAt != 0 {
			ordinary++
			if ordinary > s.state.Settings.Limit {
				removed = append(removed, item.ID)
				continue
			}
		}
		kept = append(kept, item)
	}
	s.state.Items = kept
	return removed
}

func (s *Store) List(q Query) Page {
	s.mu.Lock()
	defer s.mu.Unlock()
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 || q.PageSize > 50 {
		q.PageSize = 20
	}
	out := Page{Items: []Summary{}, Page: q.Page, PageSize: q.PageSize, Settings: PublicSettings(s.state.Settings), Revision: s.state.Revision}
	if s.lastError != nil {
		out.StorageError = s.lastError.Error()
	}
	for _, item := range s.state.Items {
		if item.Pinned {
			out.PinnedCount++
		}
		if !matches(item, q) {
			continue
		}
		out.Total++
		if out.Total > (q.Page-1)*q.PageSize && len(out.Items) < q.PageSize {
			item.ToolNames = append([]string{}, item.ToolNames...)
			out.Items = append(out.Items, item)
		}
	}
	return out
}

func validID(id string) bool {
	return strings.HasPrefix(id, "tool_") && filepath.Base(id) == id && !strings.ContainsAny(id, "/\\")
}
func (s *Store) detailPath(id string) string { return filepath.Join(s.path+".d", id+".json") }
func (s *Store) Get(id string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getLocked(id)
}
func (s *Store) getLocked(id string) (Record, error) {
	if !validID(id) {
		return Record{}, ErrNotFound
	}
	found := false
	for _, item := range s.state.Items {
		if item.ID == id {
			found = true
			break
		}
	}
	if !found {
		return Record{}, ErrNotFound
	}
	raw, err := os.ReadFile(s.detailPath(id))
	if err != nil {
		return Record{}, err
	}
	var r Record
	err = json.Unmarshal(raw, &r)
	return r, err
}

func (s *Store) SetSettings(settings Settings) error {
	if settings.Limit != 20 && settings.Limit != 50 {
		return errors.New("limit must be 20 or 50")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadError != nil {
		return s.loadError
	}
	repair, err := mergeRepairSettings(s.state.Settings.Repair, settings.Repair)
	if err != nil {
		return err
	}
	settings.Repair = repair
	before := s.state
	s.state.Items = append([]Summary{}, s.state.Items...)
	s.state.Settings = settings
	removed := s.trimLocked()
	if err := s.commitLocked(removed); err != nil {
		s.state = before
		return err
	}
	return nil
}

// Clear never deletes in-flight requests. scope must explicitly select failures.
func (s *Store) Clear(scope string) error {
	if scope != "normal" && scope != "errors" {
		return errors.New("scope must be normal or errors")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadError != nil {
		return s.loadError
	}
	before := s.state
	kept := []Summary{}
	removed := []string{}
	for _, item := range s.state.Items {
		if item.CompletedAt != 0 && (scope == "errors") == item.Pinned {
			removed = append(removed, item.ID)
		} else {
			kept = append(kept, item)
		}
	}
	s.state.Items = kept
	if err := s.commitLocked(removed); err != nil {
		s.state = before
		return err
	}
	return nil
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadError != nil {
		return s.loadError
	}
	before := s.state
	kept := []Summary{}
	found := false
	for _, item := range s.state.Items {
		if item.ID == id {
			if item.CompletedAt == 0 {
				return errors.New("cannot delete an active diagnostic")
			}
			found = true
		} else {
			kept = append(kept, item)
		}
	}
	if !found {
		return ErrNotFound
	}
	s.state.Items = kept
	if err := s.commitLocked([]string{id}); err != nil {
		s.state = before
		return err
	}
	return nil
}

func (s *Store) saveIndexLocked() error {
	next := s.state
	next.Revision = time.Now().UnixNano()
	if err := atomicJSON(s.path, next); err != nil {
		return err
	}
	s.state.Revision = next.Revision
	return nil
}
func (s *Store) commitLocked(removed []string) error {
	if err := s.saveIndexLocked(); err != nil {
		s.lastError = err
		return err
	}
	s.lastError = nil
	for _, id := range removed {
		if err := os.Remove(s.detailPath(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
			s.lastError = fmt.Errorf("remove diagnostic detail: %w", err)
		}
	}
	return nil
}
