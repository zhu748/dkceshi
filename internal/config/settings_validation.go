package config

import (
	"fmt"
	"strconv"
	"strings"
)

// CoerceSettingField validates one value coming from the admin settings API or
// a config import, and returns the canonical Go value to store. A nil value is
// reported as absent rather than coerced, so a partial update never resets a
// field it did not mention. A value that is present is validated as the value
// the user set: the "0 means unset" rule belongs to a *stored* config file and
// is applied on read (settingValue), not here.
func CoerceSettingField(spec SettingSpec, f FieldSpec, raw any) (any, bool, error) {
	if raw == nil {
		return nil, false, nil
	}
	name := spec.Key + "." + f.Name
	switch f.Kind {
	case FieldBool:
		v, err := coerceBool(raw)
		if err != nil {
			return nil, false, fmt.Errorf("%s must be a boolean", name)
		}
		return v, true, nil
	case FieldInt:
		v, err := coerceInt(raw)
		if err != nil {
			return nil, false, fmt.Errorf("%s must be an integer", name)
		}
		// The request holds this field, so 0 is a value the user chose, not the
		// "unset" sentinel: it is range-checked like any other. A field whose
		// declared minimum is positive therefore rejects an explicit 0, while
		// one whose range includes 0 (input_file.min_chars) keeps accepting it.
		if err := ValidateIntRange(name, v, f.Min, f.Max, true); err != nil {
			return nil, false, err
		}
		return v, true, nil
	case FieldString:
		// A trimmed field cannot distinguish "the user cleared it" from "the
		// user typed spaces", so the two behave identically: both are the unset
		// (empty) value. Rejecting the second would fail the whole settings
		// request over a value that means the same as an accepted one.
		str := strings.TrimSpace(fmt.Sprintf("%v", raw))
		if f.Validate != nil && str != "" {
			if err := f.Validate(str); err != nil {
				return nil, false, err
			}
		}
		return str, true, nil
	}
	return nil, false, fmt.Errorf("%s has unknown kind %q", name, f.Kind)
}

func coerceBool(raw any) (bool, error) {
	switch v := raw.(type) {
	case bool:
		return v, nil
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(v))
		if err != nil {
			return false, err
		}
		return parsed, nil
	}
	return false, fmt.Errorf("unsupported bool value %T", raw)
}

func coerceInt(raw any) (int, error) {
	switch v := raw.(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		if v != float64(int(v)) {
			return 0, fmt.Errorf("not an integer: %v", v)
		}
		return int(v), nil
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return 0, fmt.Errorf("empty")
		}
		return strconv.Atoi(trimmed)
	}
	return 0, fmt.Errorf("unsupported int value %T", raw)
}

// ValidateRegisteredSettings checks every stored value of every registered
// setting, so a config file that carries an out-of-range value is rejected at
// load rather than at the point of use.
func ValidateRegisteredSettings(c Config) error {
	for _, spec := range SettingSpecs() {
		stored := c.Settings[spec.Key]
		for _, f := range spec.Fields {
			v, ok := stored[f.Name]
			if !ok {
				continue
			}
			switch f.Kind {
			case FieldInt:
				n, ok := v.(int)
				if !ok {
					return fmt.Errorf("%s.%s must be an integer", spec.Key, f.Name)
				}
				if err := ValidateIntRange(spec.Key+"."+f.Name, n, f.Min, f.Max, false); err != nil {
					return err
				}
			case FieldString:
				str, ok := v.(string)
				if !ok {
					return fmt.Errorf("%s.%s must be a string", spec.Key, f.Name)
				}
				if f.NonBlank {
					if err := ValidateTrimmedString(spec.Key+"."+f.Name, str, false); err != nil {
						return err
					}
				}
				if f.Validate != nil && strings.TrimSpace(str) != "" {
					if err := f.Validate(strings.TrimSpace(str)); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// ValidateKeepAliveInterval checks that an interval string conforms to "X" or "X-Y" (in hours)
// and that all values lie within [0.05, 720] hours.
func ValidateKeepAliveInterval(v any) error {
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf("keep_alive.interval 必须为字符串")
	}
	raw := strings.TrimSpace(strings.ToLower(s))
	raw = strings.TrimSuffix(raw, "hours")
	raw = strings.TrimSuffix(raw, "hour")
	raw = strings.TrimSuffix(raw, "h")
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	const minHours = 0.05
	const maxHours = 720.0

	parts := strings.Split(raw, "-")
	if len(parts) == 1 {
		val, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		if err != nil || val <= 0 {
			return fmt.Errorf("keep_alive.interval 格式无效: %q，支持格式如 \"3\" 或 \"2-4\"（单位：小时）", s)
		}
		if val < minHours || val > maxHours {
			return fmt.Errorf("keep_alive.interval 取值超出有效范围 [%.2f, %d] 小时: %v", minHours, int(maxHours), val)
		}
		return nil
	}

	if len(parts) == 2 {
		minStr := strings.TrimSpace(parts[0])
		maxStr := strings.TrimSpace(parts[1])
		minVal, err1 := strconv.ParseFloat(minStr, 64)
		maxVal, err2 := strconv.ParseFloat(maxStr, 64)
		if err1 != nil || err2 != nil || minVal <= 0 || maxVal <= 0 {
			return fmt.Errorf("keep_alive.interval 范围格式无效: %q，支持格式如 \"2-4\"（单位：小时）", s)
		}
		if minVal < minHours || maxVal > maxHours {
			return fmt.Errorf("keep_alive.interval 取值超出有效范围 [%.2f, %d] 小时: %q", minHours, int(maxHours), s)
		}
		if minVal > maxVal {
			return fmt.Errorf("keep_alive.interval 范围下限不能大于上限: %q", s)
		}
		return nil
	}

	return fmt.Errorf("keep_alive.interval 格式无效: %q，支持格式如 \"3\" 或 \"2-4\"（单位：小时）", s)
}
