package textclean

import "strings"

// Only display cleanup recognizes textual leaked status/refusal suffixes.
// Ordinary mentions, literals and every tool argument remain source data.
func contentFilterSuffix(raw string, final bool) (cut, wait bool) {
	const marker = "CONTENT_FILTER"
	n := min(len(raw), len(marker))
	if !strings.EqualFold(raw[:n], marker[:n]) {
		return false, false
	}
	if len(raw) < len(marker) {
		return false, !final
	}
	tail := raw[len(marker):]
	for _, refusal := range []string{"blocked", "你好，这个问题我暂时无法回答"} {
		if strings.HasPrefix(tail, refusal) {
			return true, false
		}
		if !final && strings.HasPrefix(refusal, tail) {
			return false, true
		}
	}
	return false, false
}
