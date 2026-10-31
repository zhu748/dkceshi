package shared

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"ds2api/internal/util"
)

var WriteJSON = util.WriteJSON
var IntFrom = util.IntFrom

func IntFromQuery(r *http.Request, key string, d int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return d
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return d
	}
	return n
}

func NilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func NilIfZero(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func MaskSecretPreview(secret string) string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return ""
	}
	if len(secret) <= 4 {
		return strings.Repeat("*", len(secret))
	}
	return secret[:2] + "****" + secret[len(secret)-2:]
}

func ToStringSlice(v any) ([]string, bool) {
	arr, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		out = append(out, strings.TrimSpace(fmt.Sprintf("%v", item)))
	}
	return out, true
}

func FieldString(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", v))
}

func FieldStringOptional(m map[string]any, key string) (string, bool) {
	v, ok := m[key]
	if !ok || v == nil {
		return "", false
	}
	return strings.TrimSpace(fmt.Sprintf("%v", v)), true
}

func FieldBoolOptional(m map[string]any, key string) (bool, bool) {
	v, ok := m[key]
	if !ok || v == nil {
		return false, false
	}
	return util.ToBool(v), true
}

// FieldIntOptionalOK 读取可选的整数字段。第二个返回值表示字段是否存在且可解析；
// JSON 数字（float64）、Go int 以及数字字符串均被接受，因此允许负值。
func FieldIntOptionalOK(m map[string]any, key string) (int, bool) {
	v, ok := m[key]
	if !ok || v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	case json.Number:
		parsed, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return int(parsed), true
	case string:
		s := strings.TrimSpace(n)
		if s == "" {
			return 0, false
		}
		parsed, err := strconv.Atoi(s)
		if err != nil {
			return 0, false
		}
		return parsed, true
	}
	return 0, false
}

// fieldInt 读取整数字段，缺失或无法解析时返回 0。
func fieldInt(m map[string]any, key string) int {
	v, ok := FieldIntOptionalOK(m, key)
	if !ok {
		return 0
	}
	return v
}

func StatusOr(v int, d int) int {
	if v == 0 {
		return d
	}
	return v
}
