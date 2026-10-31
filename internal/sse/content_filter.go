package sse

import (
	"strings"
)

func hasContentFilterStatus(chunk map[string]any) bool {
	if code, _ := chunk["code"].(string); strings.EqualFold(strings.TrimSpace(code), "content_filter") {
		return true
	}
	return hasContentFilterStatusValue(chunk)
}

func hasContentFilterStatusValue(v any) bool {
	switch x := v.(type) {
	case []any:
		for _, item := range x {
			if hasContentFilterStatusValue(item) {
				return true
			}
		}
	case map[string]any:
		if p, _ := x["p"].(string); strings.Contains(strings.ToLower(p), "status") {
			if s, _ := x["v"].(string); strings.EqualFold(strings.TrimSpace(s), "content_filter") {
				return true
			}
		}
		if code, _ := x["code"].(string); strings.EqualFold(strings.TrimSpace(code), "content_filter") {
			return true
		}
		for _, vv := range x {
			if hasContentFilterStatusValue(vv) {
				return true
			}
		}
	}
	return false
}
