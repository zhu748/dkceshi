package tooldiagnostics

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// Readers see only committed indexes and immutable detail versions, and never
// wait for another completion's disk sync or detail serialization.
func (s *Store) publishSnapshotLocked() {
	snapshot := s.state
	snapshot.Items = append([]Summary(nil), snapshot.Items...)
	s.stateCache.Store(&snapshot)
	s.publishStorageErrorLocked()
}

func (s *Store) publishStorageErrorLocked() {
	var message *string
	if s.lastError != nil {
		text := s.lastError.Error()
		message = &text
	}
	s.errorCache.Store(message)
}

func validDetailVersion(version string) bool {
	if version == "" {
		return true
	}
	_, err := uuid.Parse(version)
	return err == nil && !strings.ContainsAny(version, "/\\")
}

// Recover space left by a crash between writing a version and committing its
// index. Legacy details remain readable; only recognized orphan versions go.
func (s *Store) cleanupDetailVersions() {
	entries, err := os.ReadDir(s.path + ".d")
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		s.lastError = err
		return
	}
	retained := map[string]bool{}
	for _, item := range s.state.Items {
		retained[filepath.Base(s.versionPath(item.ID, item.DetailVersion))] = true
	}
	for _, entry := range entries {
		name := entry.Name()
		stem := strings.TrimSuffix(name, ".json")
		split := strings.LastIndexByte(stem, '.')
		if entry.IsDir() || retained[name] || stem == name || split < 0 || !validID(stem[:split]) || !validDetailVersion(stem[split+1:]) {
			continue
		}
		if err := os.Remove(filepath.Join(s.path+".d", name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			s.lastError = err
		}
	}
}
