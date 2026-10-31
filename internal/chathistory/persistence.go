package chathistory

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func (s *Store) saveLocked() error {
	s.state.Version = FileVersion
	if !isAllowedLimit(s.state.Limit) {
		s.state.Limit = DefaultLimit
	}
	s.rebuildIndexLocked()

	if err := os.MkdirAll(s.detailDir, 0o755); err != nil {
		return fmt.Errorf("create chat history detail dir: %w", err)
	}
	for _, id := range sortedDetailIDs(s.deleted) {
		path := filepath.Join(s.detailDir, id+".json")
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove stale chat history detail: %w", err)
		}
	}
	for _, id := range sortedDetailIDs(s.dirty) {
		item, ok := s.details[id]
		if !ok {
			continue
		}
		path := filepath.Join(s.detailDir, id+".json")
		// Intermediate streaming writes use compact encoding: the indented
		// form is ~1.3x the payload and this file is rewritten on every
		// progress tick while Messages/HistoryText/FinalPrompt stay unchanged.
		// Terminal states keep the indented, human-readable form. Readers use
		// json.Unmarshal and accept both shapes.
		var payload []byte
		var err error
		if item.Status == "streaming" {
			payload, err = json.Marshal(detailEnvelope{
				Version: FileVersion,
				Item:    item,
			})
		} else {
			payload, err = json.MarshalIndent(detailEnvelope{
				Version: FileVersion,
				Item:    item,
			}, "", "  ")
		}
		if err != nil {
			return fmt.Errorf("encode chat history detail: %w", err)
		}
		if err := writeFileAtomic(path, append(payload, '\n')); err != nil {
			return err
		}
	}

	payload, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode chat history index: %w", err)
	}
	if err := writeFileAtomic(s.path, append(payload, '\n')); err != nil {
		return err
	}
	s.clearPendingDetailChangesLocked()
	return nil
}

func readDetailFile(path string) (Entry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Entry{}, fmt.Errorf("read chat history detail: %w", err)
	}
	var env detailEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return Entry{}, fmt.Errorf("decode chat history detail: %w", err)
	}
	return cloneEntry(env.Item), nil
}

func writeFileAtomic(path string, body []byte) error {
	dir := filepath.Dir(path)
	if dir == "" {
		dir = "."
	}
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create chat history dir: %w", err)
		}
	}
	tmpFile, err := os.CreateTemp(dir, ".chat-history-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp chat history: %w", err)
	}
	tmpPath := tmpFile.Name()
	cleanup := func() error {
		if err := os.Remove(tmpPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove temp chat history: %w", err)
		}
		return nil
	}
	withCleanup := func(primary error, closeErr error) error {
		errs := []error{primary}
		if closeErr != nil {
			errs = append(errs, fmt.Errorf("close temp chat history: %w", closeErr))
		}
		if cleanupErr := cleanup(); cleanupErr != nil {
			errs = append(errs, cleanupErr)
		}
		return errors.Join(errs...)
	}
	if _, err := tmpFile.Write(body); err != nil {
		return withCleanup(fmt.Errorf("write temp chat history: %w", err), tmpFile.Close())
	}
	if err := tmpFile.Sync(); err != nil {
		return withCleanup(fmt.Errorf("sync temp chat history: %w", err), tmpFile.Close())
	}
	if err := tmpFile.Close(); err != nil {
		if cleanupErr := cleanup(); cleanupErr != nil {
			return errors.Join(fmt.Errorf("close temp chat history: %w", err), cleanupErr)
		}
		return fmt.Errorf("close temp chat history: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		if cleanupErr := cleanup(); cleanupErr != nil {
			return errors.Join(fmt.Errorf("promote temp chat history: %w", err), cleanupErr)
		}
		return fmt.Errorf("promote temp chat history: %w", err)
	}
	return nil
}
