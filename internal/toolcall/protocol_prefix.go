package toolcall

import "strings"

// FindToolProtocolPrefixOutsideIgnored finds branded protocol markup even when
// its local name is unknown or its closing delimiter never arrived. It is a
// visibility guard, not an additional executable tool-call syntax.
func FindToolProtocolPrefixOutsideIgnored(text string, start int) (int, string) {
	if !strings.ContainsAny(text, "<＜﹤〈") {
		return -1, ""
	}
	for i := maxInt(start, 0); i < len(text); {
		next, advanced, blocked := skipXMLIgnoredSection(text, i)
		if blocked {
			break
		}
		if advanced {
			i = next
			continue
		}
		if end, ok := markdownCodeSpanEnd(text, i); ok {
			i = end
			continue
		}
		if name := toolProtocolPrefixAt(text, i); name != "" {
			return i, name
		}
		i++
	}
	return -1, ""
}

func toolProtocolPrefixAt(text string, start int) string {
	i, ok := partialToolTagNameStart(text[start:])
	if !ok {
		return ""
	}
	i += start
	for {
		if next, ok := consumeToolMarkupSeparator(text, i); ok {
			i = next
		} else if size := toolMarkupWhitespaceLikeLenAt(text, i); size > 0 {
			i += size
		} else {
			break
		}
	}
	for _, name := range []string{"epse", "dsml"} {
		if end, ok := consumeToolKeyword(text, i, name); ok {
			ch, _ := normalizedASCIIAt(text, end)
			nameChar := (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9')
			if !nameChar {
				return name
			}
		}
	}
	return ""
}

// HasUnresolvedToolMarkup rejects an unfinished branded or canonical call.
// Literal examples remain text. DSML residue has its separate removal policy.
func HasUnresolvedToolMarkup(text string) bool {
	if !strings.ContainsAny(text, "<＜﹤〈") {
		return false
	}
	text = stripFencedCodeBlocks(text)
	for pos := 0; pos < len(text); {
		idx, protocol := FindToolProtocolPrefixOutsideIgnored(text, pos)
		if idx < 0 {
			break
		}
		if protocol == "epse" {
			if tag, ok := scanToolMarkupTagAt(text, idx); !ok || !tag.Closing {
				if _, closing := consumeToolMarkupClosingSlash(text, idx+xmlTagStartDelimiterLenAt(text, idx)); !closing {
					return true
				}
			}
		}
		pos = idx + 1
	}
	for i := 0; i < len(text); {
		next, advanced, blocked := skipXMLIgnoredSection(text, i)
		if blocked {
			break
		}
		if advanced {
			i = next
			continue
		}
		if end, ok := markdownCodeSpanEnd(text, i); ok {
			i = end
			continue
		}
		if next, ok := consumeToolMarkupLessThan(text, i); ok {
			if slash, ok := consumeToolMarkupClosingSlash(text, next); ok {
				i = slash
				continue
			}
			name, size := matchToolMarkupName(text, next, false)
			if name != "" && hasToolMarkupBoundary(text, next+size) {
				return true
			}
		}
		i++
	}
	return false
}
