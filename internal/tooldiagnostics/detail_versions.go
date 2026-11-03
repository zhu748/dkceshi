package tooldiagnostics

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"

	"ds2api/internal/config"
	"github.com/google/uuid"
)

// Immutable detail versions make the index rename the single commit point.
// A failed index update cannot overwrite the previously committed evidence.
func (s *Store) versionPath(id, version string) string {
	if version != "" {
		id += "." + version
	}
	return filepath.Join(s.path+".d", id+".json")
}

func (s *Store) Put(record Record) error {
	if s == nil {
		return nil
	}
	record = boundRecord(record)
	record.ToolNames = append([]string{}, record.ToolNames...)
	record.DetailVersion = uuid.NewString()
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadError != nil {
		return s.loadError
	}
	if !validID(record.ID) {
		return errors.New("invalid diagnostic id")
	}
	oldPath := s.detailPath(record.ID)
	newPath := s.versionPath(record.ID, record.DetailVersion)
	if err := atomicData(newPath, data); err != nil {
		s.lastError = err
		s.publishStorageErrorLocked()
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
	if err := s.commitLocked(append(s.trimLocked(), oldPath)); err != nil {
		s.state = before
		if cleanupErr := os.Remove(newPath); cleanupErr != nil && !errors.Is(cleanupErr, os.ErrNotExist) {
			config.Logger.Warn("[tool_diagnostics] remove uncommitted detail failed", "error", cleanupErr)
		}
		return err
	}
	return nil
}
