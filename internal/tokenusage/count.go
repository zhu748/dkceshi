// Package tokenusage handles upstream usage metadata independently of wire protocols.
package tokenusage

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

// MaxCount is exactly representable by both Go and JavaScript clients.
const MaxCount = 1<<53 - 1

// Count distinguishes a reported zero from missing, malformed or unsafe values.
func Count(value any) (int, bool) {
	var n int64
	switch v := value.(type) {
	case int:
		n = int64(v)
	case int64:
		n = v
	case int32:
		n = int64(v)
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > MaxCount || math.Trunc(v) != v {
			return 0, false
		}
		n = int64(v)
	case json.Number:
		return Count(string(v))
	case string:
		s := strings.TrimSpace(v)
		if s == "" || strings.IndexFunc(s, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return 0, false
		}
		parsed, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0, false
		}
		n = parsed
	default:
		return 0, false
	}
	if n < 0 || n > MaxCount || int64(int(n)) != n {
		return 0, false
	}
	return int(n), true
}

func countPointer(v any) *int {
	if n, ok := Count(v); ok {
		return &n
	}
	return nil
}
