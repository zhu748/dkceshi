package toolcall

import (
	"unicode"
	"unicode/utf8"
)

// A marker typo must not escape before its local tag name arrives. Buffer a
// short unfinished tag name, including unknown prefixes, until the normal
// scanner can classify it. The bound applies only before the local name is
// recognizable, never to attributes or long parameter bodies.
func unfinishedToolTagName(text string) bool {
	if len(text) > 128 {
		return false
	}
	i, ok := partialToolTagNameStart(text)
	if !ok {
		return false
	}
	for i < len(text) {
		if size := toolMarkupIgnorableLenAt(text, i); size > 0 {
			i += size
			continue
		}
		if next, ok := consumeToolMarkupSeparator(text, i); ok {
			i = next
			continue
		}
		r, size := utf8.DecodeRuneInString(text[i:])
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-' {
			return false
		}
		i += size
	}
	return true
}

func partialToolTagNameStart(text string) (int, bool) {
	i, ok := consumeToolMarkupLessThan(text, 0)
	if !ok {
		return 0, false
	}
	for {
		next, ok := consumeToolMarkupLessThan(text, i)
		if !ok {
			break
		}
		i = next
	}
	if next, ok := consumeToolMarkupClosingSlash(text, i); ok {
		i = next
	}
	return i, true
}

func toolMarkupPrefixContainsSlash(prefix string) bool {
	for _, r := range prefix {
		if normalizeFullwidthASCII(r) == '/' {
			return true
		}
	}
	return false
}
