package toolcall

import (
	"strings"
	"unicode/utf8"
)

// stripTrailingPartialDSMLTag removes a trailing, as-yet-unterminated DSML tag
// fragment (e.g. `<｜｜DSML｜｜ cal` whose `>` never arrived because the stream
// ended). The sieve holds such fragments in pending while the stream is open
// and used to release them verbatim at flush; the final net cuts them instead.
// A bare `<` / separator run carries no DSML evidence and is left alone, and a
// fragment inside a fenced code block is documentation, not residue.
func stripTrailingPartialDSMLTag(text string) string {
	idx := strings.LastIndexAny(text, "<＜﹤〈")
	if idx < 0 {
		return text
	}
	start := idx
	for start > 0 {
		r, size := utf8.DecodeLastRuneInString(text[:start])
		if !strings.ContainsRune("<＜﹤〈", r) {
			break
		}
		start -= size
	}
	tail := text[start:]
	if !isPartialDSMLTagPrefix(tail) || !partialDSMLTailHasEvidence(tail) {
		return text
	}
	if insideFencedCode(text, start) {
		return text
	}
	for i := 0; i < start; i++ {
		next, advanced, blocked := skipXMLIgnoredSection(text, i)
		if blocked || advanced && next > start {
			return text
		}
		if advanced {
			i = next - 1
			continue
		}
		if end, ok := markdownCodeSpanEnd(text, i); ok {
			if end > start {
				return text
			}
			i = end - 1
		}
	}
	return text[:start]
}

// partialDSMLTailHasEvidence reports whether the unterminated fragment spells
// at least part of the `dsml` literal, so a lone trailing `<` (or a separator
// run) in ordinary prose is never cut.
func partialDSMLTailHasEvidence(tail string) bool {
	i, ok := partialToolTagNameStart(tail)
	if !ok {
		return false
	}
	for {
		next, ok := consumeToolMarkupSeparator(tail, i)
		if ok {
			i = next
			continue
		}
		if spacingLen := toolMarkupWhitespaceLikeLenAt(tail, i); spacingLen > 0 {
			i += spacingLen
			continue
		}
		break
	}
	if i >= len(tail) {
		return false
	}
	if _, matched := consumeToolKeyword(tail, i, "dsml"); matched {
		return true
	}
	return hasASCIIPartialPrefixFoldAt(tail, i, "dsml")
}

// dsmlBadSpans collects the spans of DSML wrappers in text.
