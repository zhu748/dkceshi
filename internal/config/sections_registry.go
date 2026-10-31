package config

import (
	"fmt"
	"strings"
	"sync"
)

var (
	sectionMu    sync.RWMutex
	sectionSpecs = map[string]SectionSpec{}
	sectionOrder []string
)

// RegisterSection declares a section. It panics on a malformed spec: every
// rejected shape below is a programming error that would otherwise surface as a
// section that silently never loads, never saves, or renders a blank control.
func RegisterSection(spec SectionSpec) {
	key := strings.TrimSpace(spec.Key)
	if key == "" {
		panic("config: section key must not be empty")
	}
	if IsSettingKey(key) {
		panic(fmt.Sprintf("config: section key %q collides with the registered setting of the same name", key))
	}
	if spec.Decode == nil {
		panic(fmt.Sprintf("config: section %q declares no Decode, so the config file could not be read", key))
	}
	if spec.Form != nil {
		if strings.TrimSpace(spec.Form.TitleKey) == "" {
			panic(fmt.Sprintf("config: section %q declares a form without a title locale key", key))
		}
		for _, f := range spec.Form.Fields {
			if err := checkSectionField(key, f); err != nil {
				panic(err.Error())
			}
		}
		// A removed field that is also a declared one would make that field
		// impossible to write, which is the opposite of what either declaration
		// says. An empty name would match nothing and silently do nothing.
		for name := range spec.Form.RemovedFields {
			if strings.TrimSpace(name) == "" {
				panic(fmt.Sprintf("config: section %q declares a removed field with an empty name", key))
			}
			if declaredSectionField(spec.Form.Fields, name) {
				panic(fmt.Sprintf("config: section %q declares %q as both a field and a removed field", key, name))
			}
		}
	}
	sectionMu.Lock()
	defer sectionMu.Unlock()
	if _, dup := sectionSpecs[key]; dup {
		panic(fmt.Sprintf("config: section %q is already registered", key))
	}
	spec.Key = key
	sectionSpecs[key] = spec
	sectionOrder = append(sectionOrder, key)
}

// declaredSectionField reports whether fields declares name at the top level.
// Nested group fields are not considered: a removed field is a top-level key of
// the section object, which is where a settings update carries it.
func declaredSectionField(fields []SectionField, name string) bool {
	for _, f := range fields {
		if f.Name == name {
			return true
		}
	}
	return false
}

func checkSectionField(section string, f SectionField) error {
	name := strings.TrimSpace(f.Name)
	if name == "" {
		return fmt.Errorf("config: section %q has a field with an empty name", section)
	}
	if strings.TrimSpace(f.LabelKey) == "" && f.Kind != FieldGroup {
		return fmt.Errorf("config: section %q field %q must declare a label locale key", section, name)
	}
	switch f.Kind {
	case FieldGroup:
		if len(f.Fields) == 0 {
			return fmt.Errorf("config: section %q field %q is a group with no fields", section, name)
		}
		for _, nested := range f.Fields {
			if err := checkSectionField(section, nested); err != nil {
				return err
			}
		}
		return nil
	case FieldEnum:
		if len(f.Options) < 2 {
			return fmt.Errorf("config: section %q field %q is an enum with fewer than two options", section, name)
		}
		for _, opt := range f.Options {
			if strings.TrimSpace(opt.Value) == "" {
				return fmt.Errorf("config: section %q field %q has an option with an empty value", section, name)
			}
			if strings.TrimSpace(opt.LabelKey) == "" {
				return fmt.Errorf("config: section %q field %q option %q must declare a label locale key", section, name, opt.Value)
			}
		}
	case FieldBool, FieldInt, FieldString:
	default:
		return fmt.Errorf("config: section %q field %q has unknown kind %q", section, name, f.Kind)
	}
	if f.Read == nil {
		return fmt.Errorf("config: section %q field %q declares no Read, so the settings API could not report it", section, name)
	}
	if f.Apply == nil {
		return fmt.Errorf("config: section %q field %q declares no Apply, so a settings update could not write it", section, name)
	}
	return nil
}

// SectionSpecs returns the registered sections in declaration order.
func SectionSpecs() []SectionSpec {
	sectionMu.RLock()
	defer sectionMu.RUnlock()
	out := make([]SectionSpec, 0, len(sectionOrder))
	for _, key := range sectionOrder {
		out = append(out, sectionSpecs[key])
	}
	return out
}

// SectionKeys returns the registered keys in declaration order.
func SectionKeys() []string {
	sectionMu.RLock()
	defer sectionMu.RUnlock()
	return append([]string(nil), sectionOrder...)
}

// LookupSection returns the declaration registered for key.
func LookupSection(key string) (SectionSpec, bool) {
	sectionMu.RLock()
	defer sectionMu.RUnlock()
	spec, ok := sectionSpecs[key]
	return spec, ok
}

// IsSectionKey reports whether key names a registered section. The codec uses it
// to decide whether a top-level config key belongs to the registry, and
// RegisterSetting uses it to refuse a name a section already owns.
func IsSectionKey(key string) bool {
	sectionMu.RLock()
	defer sectionMu.RUnlock()
	_, ok := sectionSpecs[key]
	return ok
}

// forEachSection walks the registry without copying it, for the codec paths that
// run on every config load and every snapshot.
func forEachSection(fn func(SectionSpec)) {
	sectionMu.RLock()
	defer sectionMu.RUnlock()
	for _, key := range sectionOrder {
		fn(sectionSpecs[key])
	}
}
