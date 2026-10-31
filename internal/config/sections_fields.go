package config

import (
	"fmt"
	"slices"
	"strings"
)

// SectionGroup declares a nested object field of a section form. The group's own
// fields are declared inside it, and both the settings read and the settings
// write walk them, so a nested object needs no code of its own.
//
// An empty labelKey declares a group that has no header: its fields render into
// the card, which is what a sub-feature whose switch already carries the name
// wants (see SectionField.LabelKey).
func SectionGroup(path, labelKey string, fields ...SectionField) SectionField {
	return SectionField{
		Name:     sectionFieldName(path),
		Kind:     FieldGroup,
		LabelKey: labelKey,
		Fields:   fields,
	}
}

// SectionBool declares a boolean field of a section form: it reads through
// read and writes through apply, which is called only when a request carries
// the field. path is the field's full name ("runtime.mute_report.enabled"),
// which is what the config file, the web UI and the error messages use.
func SectionBool(path, labelKey string, read func(s *Store) bool, apply func(c *Config, v bool)) SectionField {
	return SectionField{
		Name:     sectionFieldName(path),
		Kind:     FieldBool,
		LabelKey: labelKey,
		Read:     func(s *Store) any { return read(s) },
		Apply: func(c *Config, raw any) error {
			v, err := coerceBool(raw)
			if err != nil {
				return fmt.Errorf("%s must be a boolean", path)
			}
			apply(c, v)
			return nil
		},
	}
}

// SectionInt declares a bounded integer field of a section form. A value a
// request carries must be an integer inside [min, max]; zero is not accepted as
// "unset" here, because a field the request mentions is a field the user set.
func SectionInt(path, labelKey string, min, max int, read func(s *Store) int, apply func(c *Config, v int)) SectionField {
	return SectionField{
		Name:     sectionFieldName(path),
		Kind:     FieldInt,
		LabelKey: labelKey,
		Min:      min,
		Max:      max,
		Read:     func(s *Store) any { return read(s) },
		Apply: func(c *Config, raw any) error {
			v, err := coerceInt(raw)
			if err != nil {
				return fmt.Errorf("%s must be an integer", path)
			}
			if err := ValidateIntRange(path, v, min, max, true); err != nil {
				return err
			}
			apply(c, v)
			return nil
		},
	}
}

// SectionOptionalInt declares an integer field whose "unset" state is a state of
// its own: read reports (value, ok), and a field without a value is sent to the
// web UI as null, which renders an empty box rather than a 0 the user never
// typed (the web UI's IntFieldInput shows null as empty and leaves it out of the
// save payload, so an empty box keeps the stored value instead of writing one).
// Use it for a setting whose default is "not filled in", where a 0 would read as
// a number the operator chose; a field whose default is a real number uses
// SectionInt and ships that default.
func SectionOptionalInt(path, labelKey string, min, max int, read func(s *Store) (int, bool), apply func(c *Config, v int)) SectionField {
	field := SectionInt(path, labelKey, min, max, func(s *Store) int {
		v, _ := read(s)
		return v
	}, apply)
	field.Read = func(s *Store) any {
		v, ok := read(s)
		if !ok {
			return nil
		}
		return v
	}
	return field
}

// SectionString declares a free-text field of a section form. The value is
// trimmed, which is what every hand-written accessor for such a field does.
func SectionString(path, labelKey string, read func(s *Store) string, apply func(c *Config, v string)) SectionField {
	return SectionField{
		Name:     sectionFieldName(path),
		Kind:     FieldString,
		LabelKey: labelKey,
		Read:     func(s *Store) any { return read(s) },
		Apply: func(c *Config, raw any) error {
			apply(c, strings.TrimSpace(fmt.Sprintf("%v", raw)))
			return nil
		},
	}
}

// SectionEnum declares a field whose value is one of a declared set. A
// non-empty value that is not one of the options is rejected here, so the
// section's apply hook only has to decide what an empty value means: some enums
// accept "unset" and some do not.
func SectionEnum(path, labelKey string, options []SectionOption, read func(s *Store) string, apply func(c *Config, v string) error) SectionField {
	values := make([]string, 0, len(options))
	for _, opt := range options {
		values = append(values, opt.Value)
	}
	allowed := strings.Join(values, ", ")
	return SectionField{
		Name:     sectionFieldName(path),
		Kind:     FieldEnum,
		LabelKey: labelKey,
		Options:  options,
		Read:     func(s *Store) any { return read(s) },
		Apply: func(c *Config, raw any) error {
			v := normalizedFieldValue(raw)
			if v != "" && !slices.Contains(values, v) {
				return fmt.Errorf("%s must be one of %s", path, allowed)
			}
			return apply(c, v)
		},
	}
}

// withRenamedValues attaches renamed values to a declared field, so a field
// helper's call stays one expression (the same shape as withHelp for help keys).
// Keys are normalized the way the value is compared, so a declaration does not
// have to spell the old name in the casing the enum check uses.
func withRenamedValues(f SectionField, renamed map[string]string) SectionField {
	if len(renamed) == 0 {
		return f
	}
	normalized := make(map[string]string, len(renamed))
	for old, current := range renamed {
		normalized[normalizedFieldValue(old)] = current
	}
	f.RenamedValues = normalized
	return f
}

// normalizedFieldValue reduces an incoming field value to the form every enum
// comparison uses: a trimmed, lowercased string.
func normalizedFieldValue(raw any) string {
	return strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", raw)))
}

// sectionFieldName is the field's JSON key inside its section: the last segment
// of its declared path.
func sectionFieldName(path string) string {
	if idx := strings.LastIndex(path, "."); idx >= 0 {
		return path[idx+1:]
	}
	return path
}
