package config

import (
	"context"
	"errors"
	"strings"
)

// CommitGeneratedDeviceID checks the exact device/routing state used by the
// generator while holding the store lock. A filesystem promotion is rolled
// back, together with memory, when config persistence fails.
func (s *Store) CommitGeneratedDeviceID(ctx context.Context, snapshot DeviceSnapshot, deviceID string, promote func() (func() error, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(deviceID) == "" {
		return errors.New("devid: generator returned an empty device ID")
	}
	expected := snapshot.Account
	idx, ok := s.findAccountIndexLocked(expected.Identifier())
	if !ok {
		return errors.New("account not found")
	}
	current := s.cfg.Accounts[idx]
	if s.deviceVersions[expected.Identifier()] != snapshot.Version || !sameGenerationState(expected, current) || generationProxy(s.cfg, current) != snapshot.Proxy {
		return errors.New("devid: account device or routing changed during generation; retry with the current profile")
	}
	rollback, err := promote()
	if err != nil {
		return err
	}
	wasDirty := s.cfgDirty
	s.cfg.Accounts[idx].DeviceID = strings.TrimSpace(deviceID)
	s.cfg.Accounts[idx].DeviceIDType = DeviceIDTypeAuto
	s.cfgDirty = true
	if err := s.saveLocked(); err != nil {
		s.cfg.Accounts[idx] = current
		s.cfgDirty = wasDirty
		return errors.Join(err, rollback())
	}
	return nil
}
