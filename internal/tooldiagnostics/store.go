package tooldiagnostics

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var ErrNotFound = errors.New("tool diagnostic record not found")

// Only summaries reside in the store; retained failures' bodies are loaded on
// demand. mu serializes disk commits as well as retention decisions.
type Store struct {
	settingsCache atomic.Pointer[Settings]
	stateCache    atomic.Pointer[Index]
	errorCache    atomic.Pointer[string]
	mu            sync.Mutex
	path          string
	state         Index
	lastError     error
	loadError     error
}

func (s *Store) Enabled() bool {
	if s == nil {
		return false
	}
	settings := s.settingsCache.Load()
	return settings != nil && settings.Enabled
}

func (s *Store) trimLocked() []string {
	kept := make([]Summary, 0, len(s.state.Items))
	removed := []string{}
	ordinary := 0
	for _, item := range s.state.Items {
		if !item.Pinned && item.CompletedAt != 0 {
			ordinary++
			if ordinary > s.state.Settings.Limit {
				removed = append(removed, s.detailPath(item.ID))
				continue
			}
		}
		kept = append(kept, item)
	}
	s.state.Items = kept
	return removed
}

func (s *Store) List(q Query) Page {
	state := s.stateCache.Load()
	if state == nil {
		return Page{Items: []Summary{}}
	}
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 || q.PageSize > 50 {
		q.PageSize = 20
	}
	out := Page{Items: []Summary{}, Page: q.Page, PageSize: q.PageSize, Settings: PublicSettings(state.Settings), Revision: state.Revision}
	if health := s.errorCache.Load(); health != nil {
		out.StorageError = *health
	}
	for _, item := range state.Items {
		out.StoredTextBytes += int64(item.StoredTextBytes)
		if item.Pinned {
			out.PinnedCount++
			out.PinnedTextBytes += int64(item.StoredTextBytes)
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
func (s *Store) detailPath(id string) string {
	for _, item := range s.state.Items {
		if item.ID == id {
			return s.versionPath(id, item.DetailVersion)
		}
	}
	return s.versionPath(id, "")
}
func (s *Store) Get(id string) (Record, error) {
	index := s.stateCache.Load()
	if index == nil {
		return Record{}, ErrNotFound
	}
	record, err := s.getIndexed(index, id)
	// A commit may have replaced and removed the old immutable version.
	if errors.Is(err, os.ErrNotExist) {
		return s.getIndexed(s.stateCache.Load(), id)
	}
	return record, err
}
func (s *Store) getLocked(id string) (Record, error) { return s.getIndexed(&s.state, id) }
func (s *Store) getIndexed(index *Index, id string) (Record, error) {
	summary, err := indexedSummary(index, id)
	if err != nil {
		return Record{}, err
	}
	raw, err := os.ReadFile(s.versionPath(id, summary.DetailVersion))
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
	if settings.Repair != nil {
		repair, err := mergeRepairSettings(s.state.Settings.Repair, settings.Repair)
		if err != nil {
			return err
		}
		settings.Repair, settings.RepairError = repair, ""
	} else {
		settings.Repair, settings.RepairError = s.state.Settings.Repair, s.state.Settings.RepairError
	}
	before := s.state
	s.state.Items = append([]Summary{}, s.state.Items...)
	s.state.Settings = settings
	removed := s.trimLocked()
	if err := s.commitLocked(removed); err != nil {
		s.state = before
		return err
	}
	s.publishSettingsLocked()
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
			removed = append(removed, s.detailPath(item.ID))
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
	detailPath := s.detailPath(id)
	s.state.Items = kept
	if err := s.commitLocked([]string{detailPath}); err != nil {
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
		s.publishStorageErrorLocked()
		return err
	}
	s.lastError = nil
	// Publish the new manifest before deleting superseded versions, so a
	// reader whose old path disappeared can immediately retry the new one.
	s.publishSnapshotLocked()
	for _, path := range removed {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			s.lastError = fmt.Errorf("remove diagnostic detail: %w", err)
		}
	}
	s.publishStorageErrorLocked()
	return nil
}
