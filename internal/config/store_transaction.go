package config

import (
	"errors"
	"os"
)

func (s *Store) Replace(cfg Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg = cfg.Clone()
	cfg.NormalizeCredentials()
	InvalidateChangedDeviceIDs(s.cfg, &cfg)
	return s.commitConfigLocked(cfg, true)
}

func (s *Store) Update(mutator func(*Config) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	base := s.cfg.Clone()
	cfg := base.Clone()
	if err := mutator(&cfg); err != nil {
		return err
	}
	cfg.ReconcileCredentials(base)
	cfg.NormalizeCredentials()
	InvalidateChangedDeviceIDs(base, &cfg)
	return s.commitConfigLocked(cfg, false)
}

// The store lock hides the candidate until every write succeeds. Rebuild derived
// indexes and generation versions only after committing, never on failed writes.
func (s *Store) commitConfigLocked(cfg Config, replace bool) error {
	candidate := &Store{cfg: cfg, path: s.path, fromEnv: s.fromEnv,
		cfgDirty: s.cfgDirty || replace, mihomoDirty: s.mihomoDirty || replace,
		mihomoMigratePending: s.mihomoMigratePending}
	candidate.markDirty(s.cfg, cfg)
	if err := candidate.persistCandidate(); err != nil {
		return err
	}
	s.reconcileDeviceVersions(s.cfg, cfg)
	s.cfg, s.fromEnv = cfg, candidate.fromEnv
	s.cfgDirty, s.mihomoDirty = candidate.cfgDirty, candidate.mihomoDirty
	s.mihomoMigratePending = candidate.mihomoMigratePending
	s.rebuildIndexes()
	return nil
}

// Files are atomic individually. Restore a changed subscription file if the
// subsequent main config promotion fails, preserving consistency on I/O errors.
func (s *Store) persistCandidate() error {
	fileBacked := !s.fromEnv || (!IsVercel() && envWritebackEnabled())
	writesSubscriptions := (s.mihomoDirty || s.mihomoMigratePending) && fileBacked
	var previous []byte
	var readErr error
	if writesSubscriptions {
		previous, readErr = os.ReadFile(MihomoSubscriptionsPath())
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return readErr
		}
	}
	if err := s.saveLocked(); err != nil {
		if writesSubscriptions && !s.mihomoDirty {
			var rollbackErr error
			if readErr == nil {
				rollbackErr = writeConfigBytes(MihomoSubscriptionsPath(), previous)
			} else {
				rollbackErr = os.Remove(MihomoSubscriptionsPath())
				if errors.Is(rollbackErr, os.ErrNotExist) {
					rollbackErr = nil
				}
			}
			return errors.Join(err, rollbackErr)
		}
		return err
	}
	return nil
}
