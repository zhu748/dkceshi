package toolcall

import (
	"strings"
	"unicode/utf8"
)

// History uses exact delimiters and the same bounded literal state as streaming.
// JSON strings, Markdown examples, comments and CDATA remain caller-owned data.
func replaceToolMarkupMarker(text, from, to string) string {
	if !strings.Contains(text, from) {
		return text
	}
	var out strings.Builder
	out.Grow(len(text))
	var literal LiteralState
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		structural := literal.Feed(normalizeFullwidthASCII(r))
		matched := false
		if structural && r == '<' {
			for _, prefix := range []string{"<|", "</|"} {
				old := prefix + from + "|"
				if !strings.HasPrefix(text[i:], old) {
					continue
				}
				out.WriteString(prefix + to + "|")
				for _, ch := range old[1:] {
					literal.Feed(normalizeFullwidthASCII(ch))
				}
				i += len(old)
				matched = true
				break
			}
		}
		if !matched {
			out.WriteString(text[i : i+size])
			i += size
		}
	}
	return out.String()
}
