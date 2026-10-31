package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func envWritebackEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("DS2API_ENV_WRITEBACK")))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func (s *Store) IsEnvWritebackEnabled() bool {
	return envWritebackEnabled()
}

func (s *Store) HasEnvConfigSource() bool {
	rawCfg := strings.TrimSpace(os.Getenv("DS2API_CONFIG_JSON"))
	return rawCfg != ""
}

func (s *Store) ConfigPath() string {
	return s.path
}

func writeConfigFile(path string, cfg Config) error {
	persistCfg := cfg.Clone()
	persistCfg.ClearAccountTokens()
	if err := saveMihomoSubscriptionsFile(persistCfg.Mihomo); err != nil {
		return err
	}
	b, err := json.MarshalIndent(persistCfg, "", "  ")
	if err != nil {
		return err
	}
	return writeConfigBytes(path, b)
}

// writeConfigBytes replaces the config file atomically (temp file + fsync +
// rename). config.json carries account passwords, the admin password hash and
// the Vercel token, so a crash mid-write must never leave a truncated file
// behind: readers either see the complete old content or the complete new one.
// The temp file is created with 0o600 (CreateTemp default) and the promoted
// file keeps 0o644 to match the historical on-disk mode.
func writeConfigBytes(path string, b []byte) error {
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("mkdir config dir: %w", err)
	}
	tmpFile, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmpPath := tmpFile.Name()
	cleanup := func() {
		if err := os.Remove(tmpPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			Logger.Warn("[config] remove temp config failed", "path", tmpPath, "error", err)
		}
	}
	if _, err := tmpFile.Write(b); err != nil {
		if closeErr := tmpFile.Close(); closeErr != nil {
			Logger.Warn("[config] close temp config failed", "error", closeErr)
		}
		cleanup()
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		if closeErr := tmpFile.Close(); closeErr != nil {
			Logger.Warn("[config] close temp config failed", "error", closeErr)
		}
		cleanup()
		return fmt.Errorf("sync temp config: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close temp config: %w", err)
	}
	// Preserve the destination's existing permission bits when the file is
	// already there: operators may have tightened config.json (it holds account
	// passwords) to 0600, and forcing 0644 on every save would silently widen
	// it again. New files keep the historical 0644 default.
	mode := os.FileMode(0o644)
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		Logger.Warn("[config] chmod temp config failed", "path", tmpPath, "error", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return fmt.Errorf("promote temp config: %w", err)
	}
	return nil
}
