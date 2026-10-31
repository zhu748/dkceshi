package config

import (
	"encoding/json"
)

// decodeRegisteredSection hands one config-file key to its section. handled
// reports whether the key is registered at all, which is what keeps an unknown
// key falling through to AdditionalFields.
func decodeRegisteredSection(c *Config, key string, raw json.RawMessage) (handled bool, err error) {
	spec, ok := LookupSection(key)
	if !ok {
		return false, nil
	}
	if spec.Decode == nil {
		return true, nil
	}
	return true, spec.Decode(c, raw)
}

// encodeRegisteredSections writes every registered section that reports itself
// as set. A section at its zero value stays out of the file, so an untouched
// config keeps the shape it had before the section existed.
func encodeRegisteredSections(m map[string]any, c Config) {
	forEachSection(func(spec SectionSpec) {
		if spec.Encode == nil {
			return
		}
		if value, ok := spec.Encode(c); ok {
			m[spec.Key] = value
		}
	})
}

func cloneRegisteredSections(dst *Config, src Config) {
	forEachSection(func(spec SectionSpec) {
		if spec.Clone != nil {
			spec.Clone(dst, &src)
		}
	})
}

func normalizeRegisteredSections(c *Config) {
	forEachSection(func(spec SectionSpec) {
		if spec.Normalize != nil {
			spec.Normalize(c)
		}
	})
}

func validateRegisteredSections(c Config) error {
	for _, spec := range SectionSpecs() {
		if spec.Validate == nil {
			continue
		}
		if err := spec.Validate(c); err != nil {
			return err
		}
	}
	return nil
}
