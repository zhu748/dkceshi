package chathistory

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ds2api/internal/config"
)

func (s *Store) loadLocked() error {
	if strings.TrimSpace(s.path) == "" {
		return errors.New("chat history path is required")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil && filepath.Dir(s.path) != "." {
		return fmt.Errorf("create chat history dir: %w", err)
	}
	if err := os.MkdirAll(s.detailDir, 0o755); err != nil {
		return fmt.Errorf("create chat history detail dir: %w", err)
	}

	raw, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if saveErr := s.saveLocked(); saveErr != nil {
				config.Logger.Warn("[chat_history] bootstrap write failed", "path", s.path, "error", saveErr)
			}
			return nil
		}
		return fmt.Errorf("read chat history index: %w", err)
	}

	legacy, legacyOK, legacyErr := parseLegacy(raw)
	if legacyErr != nil {
		return legacyErr
	}
	if legacyOK {
		s.loadLegacyLocked(legacy)
		s.markInterruptedStreamingLocked()
		if err := s.saveLocked(); err != nil {
			config.Logger.Warn("[chat_history] legacy migration writeback failed", "path", s.path, "error", err)
		}
		return nil
	}

	var state File
	if err := json.Unmarshal(raw, &state); err != nil {
		return fmt.Errorf("decode chat history index: %w", err)
	}
	if state.Version == 0 {
		state.Version = FileVersion
	}
	if !isAllowedLimit(state.Limit) {
		state.Limit = DefaultLimit
	}
	s.state = cloneFile(state)
	s.details = map[string]Entry{}
	// A single corrupted or missing detail file must not take the whole module
	// down: loadLocked's error is cached in s.err and permanently disables the
	// store (Enabled() == false), which would also block the admin "clear
	// history" self-heal path. Skip the broken entry, warn, and let
	// rebuildIndexLocked drop it from the index so the next save prunes it.
	skipped := 0
	for _, item := range state.Items {
		detail, err := readDetailFile(filepath.Join(s.detailDir, item.ID+".json"))
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				config.Logger.Warn("[chat_history] detail file unreadable, skipping entry",
					"path", filepath.Join(s.detailDir, item.ID+".json"), "error", err)
				// Queue the orphan file for deletion: rebuildIndexLocked drops the
				// entry from the index, but without this the corrupt .json stays
				// on disk forever.
				s.markDetailDeletedLocked(item.ID)
			}
			skipped++
			continue
		}
		s.details[item.ID] = detail
	}
	if skipped > 0 {
		config.Logger.Warn("[chat_history] skipped unreadable detail entries", "count", skipped)
	}
	// No stream can survive a restart: mark leftover "streaming" entries as
	// interrupted so the admin UI does not show a spinner forever (and usage
	// accounting closes the entry).
	s.markInterruptedStreamingLocked()
	s.rebuildIndexLocked()
	if saveErr := s.saveLocked(); saveErr != nil {
		config.Logger.Warn("[chat_history] index rewrite failed", "path", s.path, "error", saveErr)
	}
	return nil
}

// markInterruptedStreamingLocked converts leftover "streaming" entries into
// interrupted errors. Callers are the two load paths (fresh index and legacy
// migration): no stream can survive a restart, and without this the admin UI
// shows a spinner forever and usage accounting never closes the entry.
func (s *Store) markInterruptedStreamingLocked() {
	nowMillis := time.Now().UnixMilli()
	for id, item := range s.details {
		if item.Status != "streaming" {
			continue
		}
		item.Status = "error"
		item.Error = "服务重启，流式中断"
		item.StatusCode = http.StatusServiceUnavailable
		if item.CompletedAt == 0 {
			item.CompletedAt = nowMillis
		}
		item.UpdatedAt = nowMillis
		// Bump the per-entry (and through nextRevisionLocked the global)
		// revision: the admin UI polls list/detail with If-None-Match, so
		// without this the interrupted rewrite stays invisible to already
		// open pages until a manual refresh — the ETag would keep matching
		// the pre-restart revision forever.
		item.Revision = s.nextRevisionLocked()
		s.details[id] = item
		s.markDetailDirtyLocked(id)
	}
}
