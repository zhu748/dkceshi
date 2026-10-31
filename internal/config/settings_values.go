package config

import (
	"strings"
)

// SettingValues reports the effective value of every field of a registered
// setting: the stored value when the config carries one, the declared default
// otherwise. This is what the settings API hands to the web UI, so a field the
// user never touched is reported with the value it actually behaves as.
func (s *Store) SettingValues(key string) map[string]any {
	spec, ok := LookupSetting(key)
	if !ok {
		return nil
	}
	out := make(map[string]any, len(spec.Fields))
	for _, f := range spec.Fields {
		out[f.Name] = s.settingValue(key, f)
	}
	return out
}

// SettingBool reads a bool field of a registered setting. An unregistered key
// or field reads as false rather than panicking: a stale caller after a setting
// is retired should lose the value, not take the process down.
func (s *Store) SettingBool(key, field string) bool {
	f, ok := LookupSettingField(key, field)
	if !ok {
		return false
	}
	v, _ := s.settingValue(key, f).(bool)
	return v
}

// SettingInt reads an int field of a registered setting.
func (s *Store) SettingInt(key, field string) int {
	f, ok := LookupSettingField(key, field)
	if !ok {
		return 0
	}
	v, _ := s.settingValue(key, f).(int)
	return v
}

// SettingString reads a string field of a registered setting.
func (s *Store) SettingString(key, field string) string {
	f, ok := LookupSettingField(key, field)
	if !ok {
		return ""
	}
	v, _ := s.settingValue(key, f).(string)
	return v
}

func (s *Store) settingValue(key string, f FieldSpec) any {
	if f.ForceValue != nil {
		return f.ForceValue
	}
	if s == nil {
		return f.Default
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	stored, ok := s.cfg.Settings[key][f.Name]
	if !ok {
		return f.Default
	}
	switch f.Kind {
	case FieldInt:
		// Zero means "unset", the same rule ValidateIntRange and the
		// hand-written accessors apply, so a stored 0 reads as the default
		// instead of disabling the setting.
		if n, ok := stored.(int); ok && n == 0 {
			return f.Default
		}
	case FieldString:
		str, _ := stored.(string)
		if f.Trim {
			return strings.TrimSpace(str)
		}
	}
	return stored
}

// SetSetting stores one field of a registered setting, creating the section on
// first write.
func (c *Config) SetSetting(key, field string, value any) {
	if c.Settings == nil {
		c.Settings = map[string]map[string]any{}
	}
	if c.Settings[key] == nil {
		c.Settings[key] = map[string]any{}
	}
	c.Settings[key][field] = value
}

// NormalizeRegisteredSettings applies the read-side normalization of every
// registered setting in place, so a stored value and the value a reader sees
// agree. It is what the hand-written NormalizeSettingsConfig used to do per
// section.
func NormalizeRegisteredSettings(c *Config) {
	if c == nil {
		return
	}
	for _, spec := range SettingSpecs() {
		for _, f := range spec.Fields {
			if f.Kind != FieldString || !f.Trim {
				continue
			}
			stored, ok := c.Settings[spec.Key][f.Name]
			if !ok {
				continue
			}
			str, ok := stored.(string)
			if !ok {
				continue
			}
			c.Settings[spec.Key][f.Name] = strings.TrimSpace(str)
		}
	}
}
