package toolcall

import (
	"unicode"
	"unicode/utf8"
)

// protocolEnvelopeAt recognizes the transport shell, independently of tool tag
// names. A damaged marker/local name is a visibility candidate, never an
// executable call. Partial shells stay quarantined until they become either a
// tool envelope or an ordinary special token such as <|assistant|>.
func protocolEnvelopeAt(text string, start int, partial bool) bool {
	i, ok := partialToolTagNameStart(text[start:])
	if !ok {
		return false
	}
	i += start
	i = skipToolMarkupIgnorables(text, i)
	if next := consumeProtocolDelimiters(text, i); next == i {
		return false
	} else {
		i = next
	}
	markerStart := i
	if r, _ := utf8.DecodeRuneInString(text[i:]); !unicode.IsLetter(r) && !unicode.IsDigit(r) {
		return false
	}
	for i < len(text) {
		if next := skipToolMarkupIgnorables(text, i); next != i {
			i = next
			continue
		}
		r, size := utf8.DecodeRuneInString(text[i:])
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-' {
			break
		}
		i += size
	}
	if i == markerStart {
		return false
	}
	if i == len(text) {
		return partial
	}
	next := consumeProtocolDelimiters(text, i)
	if next == i {
		return false
	}
	i = next
	for i < len(text) {
		if size := toolMarkupWhitespaceLikeLenAt(text, i); size > 0 {
			i += size
		} else {
			break
		}
	}
	i = skipToolMarkupIgnorables(text, i)
	if i == len(text) {
		return partial
	}
	r, _ := utf8.DecodeRuneInString(text[i:])
	return unicode.IsLetter(r) || r == '_'
}

func consumeProtocolDelimiters(text string, i int) int {
	for i < len(text) {
		pos := skipToolMarkupIgnorables(text, i)
		if pos == len(text) {
			return pos
		}
		r, size := utf8.DecodeRuneInString(text[pos:])
		if ch := normalizeFullwidthASCII(r); ch != '|' && ch != '!' && r != '、' {
			return i
		}
		i = pos + size
	}
	return i
}

// Repair may fix an unfamiliar shell only when the source still establishes a
// named invocation. The existing integrity manifest remains authoritative;
// recognizing a transport envelope never authorizes inventing a tool/argument.
func hasNamedProtocolEnvelopeIntent(text string) bool {
	for from := 0; from < len(text); {
		idx, protocol := FindToolProtocolPrefixOutsideIgnored(text, from)
		if idx < 0 {
			return false
		}
		if protocol == "envelope" {
			return HasNamedToolInvocation(text, nil)
		}
		from = idx + 1
	}
	return false
}
