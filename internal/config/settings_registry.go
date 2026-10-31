package config

import (
	"fmt"
	"strings"
	"sync"
)

var (
	settingMu     sync.RWMutex
	settingSpecs  = map[string]SettingSpec{}
	settingOrder  []string
	fieldSpecsFor = map[string]map[string]FieldSpec{}
)

// RegisterSetting declares a setting. It panics on a malformed spec: every
// rejected shape below is a programming error that would otherwise surface as a
// setting that silently never loads, never validates, or is written to the
// wrong place.
func RegisterSetting(spec SettingSpec) {
	key := strings.TrimSpace(spec.Key)
	if key == "" {
		panic("config: setting key must not be empty")
	}
	if IsSectionKey(key) {
		panic(fmt.Sprintf("config: setting key %q collides with the registered section of the same name", key))
	}
	if len(spec.Fields) == 0 {
		panic(fmt.Sprintf("config: setting %q declares no fields", key))
	}
	// A visible section is rendered by the web UI straight from this spec, so a
	// missing locale key would show up as a blank label rather than as a build
	// failure. A hidden section has no UI and needs no keys.
	if !spec.Hidden && strings.TrimSpace(spec.TitleKey) == "" {
		panic(fmt.Sprintf("config: visible setting %q must declare a title locale key", key))
	}
	fields := make(map[string]FieldSpec, len(spec.Fields))
	for _, f := range spec.Fields {
		name := strings.TrimSpace(f.Name)
		if name == "" {
			panic(fmt.Sprintf("config: setting %q has a field with an empty name", key))
		}
		if _, dup := fields[name]; dup {
			panic(fmt.Sprintf("config: setting %q declares field %q twice", key, name))
		}
		if !spec.Hidden && strings.TrimSpace(f.LabelKey) == "" {
			panic(fmt.Sprintf("config: setting %q field %q must declare a label locale key", key, name))
		}
		if err := checkFieldSpec(key, f); err != nil {
			panic(err.Error())
		}
		f.Name = name
		fields[name] = f
	}
	settingMu.Lock()
	defer settingMu.Unlock()
	if _, dup := settingSpecs[key]; dup {
		panic(fmt.Sprintf("config: setting %q is already registered", key))
	}
	spec.Key = key
	settingSpecs[key] = spec
	fieldSpecsFor[key] = fields
	settingOrder = append(settingOrder, key)
}

func checkFieldSpec(key string, f FieldSpec) error {
	switch f.Kind {
	case FieldBool:
		if _, ok := f.Default.(bool); !ok {
			return fmt.Errorf("config: setting %q field %q must declare a bool default, got %T", key, f.Name, f.Default)
		}
		if f.ForceValue != nil {
			if _, ok := f.ForceValue.(bool); !ok {
				return fmt.Errorf("config: setting %q field %q must declare a bool ForceValue, got %T", key, f.Name, f.ForceValue)
			}
		}
	case FieldInt:
		def, ok := f.Default.(int)
		if !ok {
			return fmt.Errorf("config: setting %q field %q must declare an int default, got %T", key, f.Name, f.Default)
		}
		if def != 0 && (def < f.Min || def > f.Max) {
			return fmt.Errorf("config: setting %q field %q has default %d outside [%d, %d]", key, f.Name, def, f.Min, f.Max)
		}
		if f.ForceValue != nil {
			if _, ok := f.ForceValue.(int); !ok {
				return fmt.Errorf("config: setting %q field %q must declare an int ForceValue, got %T", key, f.Name, f.ForceValue)
			}
		}
	case FieldString:
		if _, ok := f.Default.(string); !ok {
			return fmt.Errorf("config: setting %q field %q must declare a string default, got %T", key, f.Name, f.Default)
		}
		if f.ForceValue != nil {
			if _, ok := f.ForceValue.(string); !ok {
				return fmt.Errorf("config: setting %q field %q must declare a string ForceValue, got %T", key, f.Name, f.ForceValue)
			}
		}
		if f.Validate != nil && f.Default != nil {
			if str, ok := f.Default.(string); ok && str != "" {
				if err := f.Validate(str); err != nil {
					return fmt.Errorf("config: setting %q field %q default value invalid: %w", key, f.Name, err)
				}
			}
		}
	default:
		return fmt.Errorf("config: setting %q field %q has unknown kind %q", key, f.Name, f.Kind)
	}
	return nil
}

// SettingSpecs returns the registered settings in declaration order, which is
// the order the settings API and the web UI report them in.
func SettingSpecs() []SettingSpec {
	settingMu.RLock()
	defer settingMu.RUnlock()
	out := make([]SettingSpec, 0, len(settingOrder))
	for _, key := range settingOrder {
		out = append(out, settingSpecs[key])
	}
	return out
}

// LookupSetting returns the declaration registered for key.
func LookupSetting(key string) (SettingSpec, bool) {
	settingMu.RLock()
	defer settingMu.RUnlock()
	spec, ok := settingSpecs[key]
	return spec, ok
}

// LookupSettingField returns the declaration of one field of a registered
// setting.
func LookupSettingField(key, field string) (FieldSpec, bool) {
	settingMu.RLock()
	defer settingMu.RUnlock()
	spec, ok := fieldSpecsFor[key][field]
	return spec, ok
}

// LookupSettingFieldFolded returns the declaration of the field of spec that
// matches name, preferring an exact match and falling back to a match that
// differs only in case.
//
// The fallback exists because the hand-written codec decoded a section straight
// into its struct, and encoding/json matches object keys against struct tags
// case-insensitively. A config file that spells `Enabled` rather than `enabled`
// therefore used to load; without the fallback the registry lookup would drop
// the field silently and the next save would erase the section from the file.
func LookupSettingFieldFolded(spec SettingSpec, name string) (FieldSpec, bool) {
	if f, ok := LookupSettingField(spec.Key, name); ok {
		return f, true
	}
	for _, f := range spec.Fields {
		if strings.EqualFold(f.Name, name) {
			return f, true
		}
	}
	return FieldSpec{}, false
}

// IsSettingKey reports whether key names a registered setting. The codec uses
// it to decide whether a top-level config key belongs to the registry.
func IsSettingKey(key string) bool {
	settingMu.RLock()
	defer settingMu.RUnlock()
	_, ok := settingSpecs[key]
	return ok
}

// SettingKeys returns the registered keys in declaration order.
func SettingKeys() []string {
	settingMu.RLock()
	defer settingMu.RUnlock()
	return append([]string(nil), settingOrder...)
}
