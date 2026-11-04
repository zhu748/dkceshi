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
	keys      []string
	ordered   bool
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
	if cap(s.checkpointBuffer) < len(s.entries) {
		s.checkpointBuffer = make([]Entry, 0, len(s.entries))
	}
	payload := s.snapshotLocked()
	s.mu.Unlock()

	writeErr := writeSnapshot(s.path, payload, write)
	// The writer consumed its bytes synchronously. Keep capacity for the next
	// flush, but clear string references so removed accounts are not retained.
	clear(payload.Entries)

	s.mu.Lock()
	// Record only adds keys; import/clear take flushMu and invalidate the cache.
	// Reuse ordering even after a failed write, while keeping dirty for retry.
	if len(s.entries) == len(payload.Entries) {
		s.checkpointOrder = payload.keys
	}
	if writeErr == nil && s.revision == revision {
		s.dirty = false
	}
	s.mu.Unlock()
	return writeErr
}

func (s *Store) snapshotLocked() fileData {
	payload := fileData{
		Version:   FileVersion,
		UpdatedAt: time.Now().UnixMilli(),
		Entries:   s.checkpointBuffer[:0],
	}
	if len(s.checkpointOrder) == len(s.entries) {
		payload.ordered = true
		payload.keys = s.checkpointOrder
		for _, key := range s.checkpointOrder {
			payload.Entries = append(payload.Entries, *s.entries[key])
		}
		return payload
	}
	payload.keys = make([]string, 0, len(s.entries))
	for key, entry := range s.entries {
		payload.Entries = append(payload.Entries, *entry)
		payload.keys = append(payload.keys, key)
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
