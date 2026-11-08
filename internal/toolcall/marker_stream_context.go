package toolcall

import (
	"strings"
	"unicode/utf8"
)

// rewrite preserves literal values while normalizing only a tag's prefix.
// State is bounded: the CDATA delimiters and tag prefix can span any chunk.
func (n *MarkerNormalizer) rewrite(text string, final bool) (string, string) {
	var out strings.Builder
	out.Grow(len(text))
	for i := 0; i < len(text); {
		if !final && !utf8.FullRuneInString(text[i:]) {
			return out.String(), text[i:]
		}
		if n.tagPrefix && len(text)-i >= len(n.marker) && asciiEqualFold(text[i:i+len(n.marker)], n.marker) {
			after := i + len(n.marker)
			if !final && (after == len(text) || !utf8.FullRuneInString(text[after:])) {
				return out.String(), text[i:]
			}
			boundary := true
			if after < len(text) {
				ch, _ := utf8.DecodeRuneInString(text[after:])
				boundary = !isASCIIAlphaNumeric(normalizeFullwidthASCII(ch))
			}
			if boundary {
				out.WriteString(EPSEKeyword)
				for _, ch := range text[i:after] {
					n.observeRune(ch)
				}
				i = after
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(text[i:])
		out.WriteString(text[i : i+size])
		n.observeRune(r)
		i += size
	}
	return out.String(), ""
}

func (n *MarkerNormalizer) observeRune(r rune) {
	ch := normalizeFullwidthASCII(r)
	if !n.literal.Feed(ch) {
		n.tagPrefix = false
	} else if ch == '<' {
		n.tagPrefix = true
	} else if !n.tagPrefix || (ch != '/' && ch != '∕' && ch != '⁄' && ch != '⧸' && !isToolMarkupSeparator(r)) {
		n.tagPrefix = false
	}
}
