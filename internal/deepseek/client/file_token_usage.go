package client

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

// A nil count is unknown; zero is an authoritative upstream count.
func readFileTokenUsage(result *UploadFileResult, metadata map[string]any) {
	value, ok := nonNegativeFileTokens(metadata["token_usage"])
	if ok {
		result.TokenUsage = &value
	}
}

func nonNegativeFileTokens(raw any) (int, bool) {
	var number int64
	switch v := raw.(type) {
	case int:
		return v, v >= 0
	case int64:
		number = v
	case json.Number:
		return nonNegativeFileTokens(string(v))
	case string:
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, false
		}
		number = parsed
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v >= float64(math.MaxInt) || math.Trunc(v) != v {
			return 0, false
		}
		return int(v), true
	default:
		return 0, false
	}
	if number < 0 || uint64(number) > uint64(math.MaxInt) {
		return 0, false
	}
	return int(number), true
}

func cloneFileTokenUsage(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func mergeFileTokenUsage(dst, src *UploadFileResult) {
	if src.TokenUsage != nil {
		dst.TokenUsage = cloneFileTokenUsage(src.TokenUsage)
	}
}

func mergeUploadFileResults(dst, src *UploadFileResult) {
	if dst == nil || src == nil {
		return
	}
	if strings.TrimSpace(src.ID) != "" {
		dst.ID = strings.TrimSpace(src.ID)
	}
	if strings.TrimSpace(src.Filename) != "" {
		dst.Filename = strings.TrimSpace(src.Filename)
	}
	if src.Bytes > 0 {
		dst.Bytes = src.Bytes
	}
	if strings.TrimSpace(src.Status) != "" {
		dst.Status = strings.TrimSpace(src.Status)
	}
	if strings.TrimSpace(src.Purpose) != "" {
		dst.Purpose = strings.TrimSpace(src.Purpose)
	}
	dst.IsImage = src.IsImage
	mergeFileTokenUsage(dst, src)
	if len(src.Raw) > 0 {
		dst.Raw = src.Raw
	}
	if src.RawHeaders != nil {
		dst.RawHeaders = src.RawHeaders.Clone()
	}
}

func firstPositiveInt64(m map[string]any, keys ...string) int64 {
	for _, key := range keys {
		if v := toInt64(m[key], 0); v > 0 {
			return v
		}
	}
	return 0
}
