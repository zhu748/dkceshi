package promptcompat

import (
	"encoding/json"
	"unicode/utf8"
)

// Preflight decoded HTTP JSON before allocating an encoded copy. Checking the
// size after Marshal still allocated an entire large attachment for tracing.
func marshalSourceField(value any, limit int) ([]byte, bool, error) {
	budget := sourceJSONBudget{remaining: limit}
	if !budget.value(value, 0) {
		return nil, false, nil
	}
	data, err := json.Marshal(value)
	return data, len(data) <= limit, err
}

type sourceJSONBudget struct{ remaining int }

func (b *sourceJSONBudget) add(n int) bool {
	if n > b.remaining {
		return false
	}
	b.remaining -= n
	return true
}

func (b *sourceJSONBudget) value(value any, depth int) bool {
	if depth > 10000 {
		return false
	}
	switch value := value.(type) {
	case string:
		return b.text(value)
	case map[string]any:
		if value == nil {
			return b.add(4) // null, not an empty object.
		}
		if !b.add(2 + max(0, len(value)-1)) {
			return false
		}
		for key, child := range value {
			if !b.text(key) || !b.add(1) || !b.value(child, depth+1) {
				return false
			}
		}
		return true
	case []any:
		if value == nil {
			return b.add(4) // null, not an empty array.
		}
		if !b.add(2 + max(0, len(value)-1)) {
			return false
		}
		for _, child := range value {
			if !b.value(child, depth+1) {
				return false
			}
		}
		return true
	case json.Number:
		return b.add(len(value))
	default:
		// HTTP input reaches here only for small scalar values. Keep support
		// for typed internal callers without changing encoding/json semantics.
		data, err := json.Marshal(value)
		return err != nil || b.add(len(data))
	}
}

func (b *sourceJSONBudget) text(value string) bool {
	if !b.add(len(value) + 2) {
		return false
	}
	for i := 0; i < len(value); {
		ch := value[i]
		extra, size := 0, 1
		switch {
		case ch == '"' || ch == '\\' || ch == '\n' || ch == '\r' || ch == '\t' || ch == '\b' || ch == '\f':
			extra = 1
		case ch < 32 || ch == '<' || ch == '>' || ch == '&':
			extra = 5
		case ch >= utf8.RuneSelf:
			r, n := utf8.DecodeRuneInString(value[i:])
			size = n
			if r == '\u2028' || r == '\u2029' || r == utf8.RuneError && n == 1 {
				extra = 6 - n
			}
		}
		if !b.add(extra) {
			return false
		}
		i += size
	}
	return true
}
