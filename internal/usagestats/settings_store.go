package usagestats

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"ds2api/internal/config"
)

func (s *Store) Settings() UsageSettings {
	if s == nil {
		return DefaultSettings()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneSettings(s.settings)
}

// cloneSettings 深拷贝设置，避免调用方改到 Store 内部状态。Models 表与
// 「模拟缓存命中」段的指针字段都必须重新分配。
func cloneSettings(settings UsageSettings) UsageSettings {
	out := settings
	out.Models = make(map[string]ModelPrice, len(settings.Models))
	for key, price := range settings.Models {
		copied := price
		copied.InputMissPrice = cloneFloat64Ptr(price.InputMissPrice)
		out.Models[key] = copied
	}
	out.CacheHit = CacheHitSettings{
		Enabled: cloneBoolPtr(settings.CacheHit.Enabled),
		Rate:    cloneFloat64Ptr(settings.CacheHit.Rate),
	}
	return out
}

func (s *Store) SaveSettings(settings UsageSettings) error {
	if s == nil {
		return nil
	}
	if err := validatePeakSettings(settings.Peak); err != nil {
		return err
	}
	if err := validateModelPrices(settings.Models); err != nil {
		return err
	}
	settings = normalizeSettings(settings)
	// Defaults can complete a missing window, so validate the effective pair
	// before publishing or persisting it as well as the original input.
	if err := validatePeakSettings(settings.Peak); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.settings
	s.settings = cloneSettings(settings)
	if err := s.saveSettingsLocked(); err != nil {
		s.settings = previous
		return err
	}
	return nil
}

func (s *Store) loadSettings() {
	if s == nil || s.settingsPath == "" {
		return
	}
	raw, err := os.ReadFile(s.settingsPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			config.Logger.Warn("[usage_stats] settings load failed", "error", err)
		}
		return
	}
	var settings UsageSettings
	if err := json.Unmarshal(raw, &settings); err != nil {
		config.Logger.Warn("[usage_stats] settings parse failed", "error", err)
		return
	}
	settings = normalizeSettings(settings)
	if err := validatePeakSettings(settings.Peak); err != nil {
		config.Logger.Warn("[usage_stats] settings peak invalid, using defaults", "error", err)
		settings.Peak = DefaultSettings().Peak
	}
	s.mu.Lock()
	s.settings = settings
	if err := s.saveSettingsLocked(); err != nil {
		config.Logger.Warn("[usage_stats] settings normalize/write failed", "error", err)
	}
	s.mu.Unlock()
}

func (s *Store) saveSettingsLocked() error {
	if s == nil || s.settingsPath == "" {
		return nil
	}
	raw, err := json.MarshalIndent(s.settings, "", "  ")
	if err != nil {
		return fmt.Errorf("settings marshal: %w", err)
	}
	return writeFileAtomic(s.settingsPath, append(raw, '\n'))
}
