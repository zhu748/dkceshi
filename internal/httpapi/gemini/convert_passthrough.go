package gemini

import (
	"encoding/json"
	"strings"
)

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func stringifyJSON(v any) string {
	switch x := v.(type) {
	case nil:
		return "{}"
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return "{}"
		}
		return s
	default:
		b, err := json.Marshal(x)
		if err != nil || len(b) == 0 {
			return "{}"
		}
		return string(b)
	}
}
