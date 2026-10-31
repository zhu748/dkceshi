package config

import (
	"encoding/json"
	"fmt"
	"sort"
)

// SectionValues reports the effective value of every field of a registered
// section: what the settings API hands to the web UI, and what a client reads
// back. Each field is read through its own reader, so a value the config does
// not carry is reported as the value it behaves as rather than as zero.
func (s *Store) SectionValues(key string) map[string]any {
	spec, ok := LookupSection(key)
	if !ok || spec.Form == nil {
		return nil
	}
	out := map[string]any{}
	readSectionFields(s, spec.Form.Fields, out)
	if spec.Form.Extra != nil {
		for name, value := range spec.Form.Extra(s) {
			out[name] = value
		}
	}
	return out
}

func readSectionFields(s *Store, fields []SectionField, out map[string]any) {
	for _, f := range fields {
		if f.Kind == FieldGroup {
			group := map[string]any{}
			readSectionFields(s, f.Fields, group)
			out[f.Name] = group
			continue
		}
		if f.Read != nil {
			out[f.Name] = f.Read(s)
		}
	}
}

// SectionUpdates reports the sections a settings update carries, so the write
// path knows what to apply and which post-write hooks to run without naming any
// section. A key the registry does not declare is ignored, which is what the
// hand-written parsers did.
func SectionUpdates(req map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, spec := range SectionSpecs() {
		if spec.Form == nil {
			continue
		}
		if raw, ok := req[spec.Key].(map[string]any); ok {
			out[spec.Key] = raw
		}
	}
	return out
}

// ApplySections merges the sections a settings update carries into c. Only the
// fields the request actually holds are applied, so a partial update never
// resets a field it did not mention — the rule the hand-written parsers
// followed, now derived from the declaration. A field the section removed is
// rejected rather than ignored, so a stale client is told instead of getting a
// success for a write that changed nothing.
func ApplySections(c *Config, updates map[string]map[string]any) error {
	for _, spec := range SectionSpecs() {
		if spec.Form == nil {
			continue
		}
		raw, ok := updates[spec.Key]
		if !ok {
			continue
		}
		if err := rejectRemovedFields(spec, raw); err != nil {
			return err
		}
		if err := applySectionFields(c, spec.Key, spec.Form.Fields, raw); err != nil {
			return err
		}
	}
	return nil
}

// rejectRemovedFields refuses a settings update that still carries a field its
// section no longer declares, naming the field that replaced it. Fields are
// checked in a stable order so the error does not depend on map iteration.
func rejectRemovedFields(spec SectionSpec, raw map[string]any) error {
	if len(spec.Form.RemovedFields) == 0 {
		return nil
	}
	removed := make([]string, 0, len(spec.Form.RemovedFields))
	for name := range spec.Form.RemovedFields {
		removed = append(removed, name)
	}
	sort.Strings(removed)
	for _, name := range removed {
		if _, present := raw[name]; !present {
			continue
		}
		return fmt.Errorf("%s.%s is no longer accepted; use %s.%s instead",
			spec.Key, name, spec.Key, spec.Form.RemovedFields[name])
	}
	return nil
}

// ValidateAppliedSections checks the sections a settings update touched, on the
// merged config, so a value that is only invalid next to what the config already
// carries (global_max_inflight below account_max_inflight) is rejected before
// the update is committed.
func ValidateAppliedSections(c Config, updates map[string]map[string]any) error {
	for _, spec := range SectionSpecs() {
		if _, touched := updates[spec.Key]; !touched || spec.Validate == nil {
			continue
		}
		if err := spec.Validate(c); err != nil {
			return err
		}
	}
	return nil
}

func applySectionFields(c *Config, section string, fields []SectionField, raw map[string]any) error {
	for _, f := range fields {
		value, present := raw[f.Name]
		if !present {
			continue
		}
		if f.Kind == FieldGroup {
			nested, ok := value.(map[string]any)
			if !ok {
				return fmt.Errorf("%s.%s must be an object", section, f.Name)
			}
			if err := applySectionFields(c, section, f.Fields, nested); err != nil {
				return err
			}
			continue
		}
		if f.Apply == nil {
			continue
		}
		if renamed, ok := f.RenamedValues[normalizedFieldValue(value)]; ok {
			value = renamed
		}
		if err := f.Apply(c, value); err != nil {
			return err
		}
	}
	return nil
}

// MergeRegisteredSections folds an imported config into next. A section that
// declares Merge follows its own rule; a section with a form and no Merge is
// merged field by field, through the same appliers a settings update uses, so an
// imported section cannot drift from what the settings page saves. A section
// with neither is left alone: a record list merges through its own endpoints.
func MergeRegisteredSections(next *Config, incoming Config) error {
	for _, spec := range SectionSpecs() {
		if spec.Merge != nil {
			if err := spec.Merge(next, &incoming); err != nil {
				return err
			}
			continue
		}
		if spec.Form == nil || spec.Encode == nil {
			continue
		}
		if err := mergeSectionFields(next, incoming, spec); err != nil {
			return err
		}
	}
	return nil
}

// mergeSectionFields applies the fields the incoming section reports as set.
// The incoming config is encoded by the section's own encoder, so "what the
// import carries" is exactly "what the config file would hold" — a field at its
// zero value is absent and is therefore not imported over the current value.
func mergeSectionFields(next *Config, incoming Config, spec SectionSpec) error {
	encoded, ok := spec.Encode(incoming)
	if !ok {
		return nil
	}
	raw, err := json.Marshal(encoded)
	if err != nil {
		return err
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		// The section is not a JSON object (a scalar or a list); there is no
		// field-wise merge to do.
		return nil
	}
	return applySectionFields(next, spec.Key, spec.Form.Fields, fields)
}
