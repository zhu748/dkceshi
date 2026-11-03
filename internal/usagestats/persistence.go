package usagestats

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"ds2api/internal/config"
)

type fileData struct {
	Version   int     `json:"version"`
	UpdatedAt int64   `json:"updated_at"`
	Entries   []Entry `json:"entries"`
}

func (s *Store) startFlusher() {
	s.stop = make(chan struct{})
	s.done = make(chan struct{})
	go s.flushLoop()
}

func (s *Store) flushLoop() {
	ticker := time.NewTicker(FlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			if err := s.flushNow(); err != nil {
				config.Logger.Warn("[usage_stats] final flush failed", "path", s.path, "error", err)
			}
			close(s.done)
			return
		case <-ticker.C:
			if err := s.flushNow(); err != nil {
				config.Logger.Warn("[usage_stats] save failed", "path", s.path, "error", err)
			}
		}
	}
}

func (s *Store) flushNow() error {
	return s.flushWith(writeFileAtomic)
}

func (s *Store) flushWith(write func(string, []byte) error) error {
	// Serialize before taking a snapshot so an older flush cannot replace a
	// newer revision after that revision has already cleared the dirty flag.
	s.flushMu.Lock()
	defer s.flushMu.Unlock()
	s.mu.Lock()
	if !s.dirty {
		s.mu.Unlock()
		return nil
	}
	revision := s.revision
	payload := s.snapshotLocked()
	s.mu.Unlock()

	if err := writeSnapshot(s.path, payload, write); err != nil {
		return err
	}

	s.mu.Lock()
	if s.revision == revision {
		s.dirty = false
	}
	s.mu.Unlock()
	return nil
}

func (s *Store) snapshotLocked() fileData {
	payload := fileData{
		Version:   FileVersion,
		UpdatedAt: time.Now().UnixMilli(),
		Entries:   make([]Entry, 0, len(s.entries)),
	}
	for _, entry := range s.entries {
		payload.Entries = append(payload.Entries, *entry)
	}
	return payload
}

func writeFileAtomic(path string, body []byte) error {
	dir := filepath.Dir(path)
	if dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0o755); err != nil && dir != "." {
		return fmt.Errorf("create usage stats dir: %w", err)
	}
	tmpFile, err := os.CreateTemp(dir, ".usage-stats-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp usage stats file: %w", err)
	}
	tmpPath := tmpFile.Name()
	cleanup := func() {
		if err := os.Remove(tmpPath); err != nil && !os.IsNotExist(err) {
			config.Logger.Warn("[usage_stats] remove temporary file failed", "error", err)
		}
	}
	if _, err := tmpFile.Write(body); err != nil {
		if err := tmpFile.Close(); err != nil {
			slog.Warn("cleanup close failed", "error", err)
		}
		cleanup()
		return fmt.Errorf("write temp usage stats file: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		if err := tmpFile.Close(); err != nil {
			slog.Warn("cleanup close failed", "error", err)
		}
		cleanup()
		return fmt.Errorf("sync temp usage stats file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close temp usage stats file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return fmt.Errorf("promote temp usage stats file: %w", err)
	}
	return nil
}
