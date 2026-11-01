package sse

import (
	"strings"

	dsprotocol "ds2api/internal/deepseek/protocol"
)

func shouldSkipPath(path string) bool {
	if isFragmentStatusPath(path) {
		return true
	}
	if _, ok := dsprotocol.SkipExactPathSet[path]; ok {
		return true
	}
	for _, p := range dsprotocol.SkipContainsPatterns {
		if strings.Contains(path, p) {
			return true
		}
	}
	return false
}

// SkipPath reports whether a patch path is protocol metadata that must never be
// treated as visible content (status / heartbeat / usage frames). Exported so
// sibling layers (e.g. the auto-continue state machine) can reuse the same
// classification when deciding whether an interrupted stream produced content.
func SkipPath(path string) bool {
	return shouldSkipPath(path)
}

func isFragmentStatusPath(path string) bool {
	if path == "" || path == "response/status" {
		return false
	}
	// Fragment patch paths can be absolute, response-relative, or relative to
	// a fragments collection (e.g. -1/status in a nested BATCH).
	path = strings.TrimPrefix(path, "response/fragments/")
	path = strings.TrimPrefix(path, "fragments/")
	if !strings.HasSuffix(path, "/status") {
		return false
	}
	mid := strings.TrimSuffix(path, "/status")
	if mid == "" {
		return false
	}
	mid = strings.TrimPrefix(mid, "-")
	if mid == "" {
		return false
	}
	for _, r := range mid {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// isToolFragmentType reports whether a fragment type carries search/tool
// metadata (TOOL_SEARCH / TOOL_OPEN / ...). Its content is progress narration
// ("正在搜索图片", "已搜索到相关图片"), not the answer body, so it is routed to
// reasoning instead of visible content.
func isToolFragmentType(typeName string) bool {
	return strings.HasPrefix(typeName, "TOOL_")
}

func isStatusPath(path string) bool {
	return path == "response/status" || path == "status" || path == "response/quasi_status" || path == "quasi_status"
}

// isFragmentScopedPath reports whether a patch path targets one specific
// fragment, e.g. "response/fragments/-1" or "fragments/8". Ops nested under such
// a path are relative to that fragment: a {"p":"status","v":"FINISHED"} there
// marks the fragment finished, not the whole response. Collection appends
// ("response/fragments") and deeper paths ("fragments/-1/content") are not
// fragment-scoped array parents.
func isFragmentScopedPath(path string) bool {
	p := strings.TrimPrefix(strings.TrimSpace(path), "response/")
	if !strings.HasPrefix(p, "fragments/") {
		return false
	}
	rest := strings.TrimPrefix(p, "fragments/")
	if rest == "" {
		return false
	}
	return !strings.Contains(rest, "/")
}

func IsCitation(text string) bool {
	return strings.HasPrefix(strings.TrimSpace(text), "[citation:")
}
