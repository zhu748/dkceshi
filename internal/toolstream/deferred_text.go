package toolstream

import (
	"ds2api/internal/toolcall"
	"strings"
	"unicode/utf8"
)

func shouldDeferToolText(state *State, text string) bool {
	if !state.DeferMalformed {
		return false
	}
	epse, canonical := toolcall.ContainsToolMarkupSyntaxOutsideIgnored(text)
	return epse || canonical || toolcall.DetectToolCallIntent(text)
}

func includeDuplicateLeadingLessThan(s string, idx int) int {
	for idx > 0 {
		r, size := utf8.DecodeLastRuneInString(s[:idx])
		if !strings.ContainsRune("<＜﹤〈", r) {
			break
		}
		idx -= size
	}
	return idx
}
