package usagestats

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// quarantineUnreadableLedger renames an undecodable ledger file aside so a
// later flush cannot overwrite it. Returns the quarantine path, or "" when
// there is nothing to preserve (the file does not exist).
func quarantineUnreadableLedger(path string) (string, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	quarantined := fmt.Sprintf("%s.corrupt-%d", path, time.Now().Unix())
	if err := os.Rename(path, quarantined); err != nil {
		return "", err
	}
	return quarantined, nil
}

func (s *Store) loadLocked() error {
	if s.path == "" {
		return errors.New("usage stats path is required")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil && filepath.Dir(s.path) != "." {
		return fmt.Errorf("create usage stats dir: %w", err)
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read usage stats file: %w", err)
	}
	if len(raw) == 0 {
		return nil
	}
	if entries, ok := decodeLegacyArray(raw); ok {
		s.ingestEntries(entries)
		return nil
	}
	var loaded fileData
	if err := json.Unmarshal(raw, &loaded); err != nil {
		return fmt.Errorf("decode usage stats file: %w", err)
	}
	if len(loaded.Entries) > 0 {
		s.ingestEntries(loaded.Entries)
		return nil
	}
	if days, ok := decodeLegacyV1Days(raw); ok {
		s.entries = migrateLegacyV1Days(days)
		return nil
	}
	return nil
}
