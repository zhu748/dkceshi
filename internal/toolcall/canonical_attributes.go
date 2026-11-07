package toolcall

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func parseCanonicalToolMarkupAttrs(raw string, idx int, strict ...bool) []canonicalToolMarkupAttr {
	if raw == "" || idx >= len(raw) {
		return nil
	}
	var out []canonicalToolMarkupAttr
	for idx < len(raw) {
		idx = skipToolMarkupIgnorables(raw, idx)
		if idx >= len(raw) {
			break
		}
		if spacingLen := toolMarkupWhitespaceLikeLenAt(raw, idx); spacingLen > 0 {
			idx += spacingLen
			continue
		}
		if xmlTagEndDelimiterLenAt(raw, idx) > 0 {
			break
		}
		if next, ok := consumeToolMarkupPipe(raw, idx); ok {
			idx = next
			continue
		}
		if next, ok := consumeToolMarkupClosingSlash(raw, idx); ok {
			idx = next
			continue
		}

		keyStart := idx
		for idx < len(raw) {
			idx = skipToolMarkupIgnorables(raw, idx)
			if idx >= len(raw) {
				break
			}
			if spacingLen := toolMarkupWhitespaceLikeLenAt(raw, idx); spacingLen > 0 {
				break
			}
			if toolMarkupEqualsLenAt(raw, idx) > 0 || xmlTagEndDelimiterLenAt(raw, idx) > 0 {
				break
			}
			if _, ok := consumeToolMarkupPipe(raw, idx); ok {
				break
			}
			if _, ok := consumeToolMarkupClosingSlash(raw, idx); ok {
				break
			}
			_, size := utf8.DecodeRuneInString(raw[idx:])
			if size <= 0 {
				idx++
			} else {
				idx += size
			}
		}
		keyEnd := idx
		key := normalizeCanonicalToolAttrKey(raw[keyStart:keyEnd])
		idx = skipToolMarkupIgnorables(raw, idx)
		for {
			spacingLen := toolMarkupWhitespaceLikeLenAt(raw, idx)
			if spacingLen == 0 {
				break
			}
			idx += spacingLen
			idx = skipToolMarkupIgnorables(raw, idx)
		}
		if eqLen := toolMarkupEqualsLenAt(raw, idx); eqLen > 0 {
			idx += eqLen
		} else {
			if len(strict) > 0 && strict[0] && strings.IndexFunc(raw[keyStart:keyEnd], func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) >= 0 {
				out = append(out, canonicalToolMarkupAttr{Key: "__unsupported_attribute"})
			}
			continue
		}
		idx = skipToolMarkupIgnorables(raw, idx)
		for {
			spacingLen := toolMarkupWhitespaceLikeLenAt(raw, idx)
			if spacingLen == 0 {
				break
			}
			idx += spacingLen
			idx = skipToolMarkupIgnorables(raw, idx)
		}
		if key == "" {
			key = "__unsupported_attribute"
		}

		value := ""
		if quote, quoteLen := xmlQuotePairAt(raw, idx); quoteLen > 0 {
			valueStart := idx + quoteLen
			idx = valueStart
			for idx < len(raw) {
				if closeLen := xmlQuoteCloseDelimiterLenAt(raw, idx, quote); closeLen > 0 {
					value = raw[valueStart:idx]
					idx += closeLen
					break
				}
				_, size := utf8.DecodeRuneInString(raw[idx:])
				if size <= 0 {
					idx++
				} else {
					idx += size
				}
			}
		} else {
			valueStart := idx
			for idx < len(raw) {
				if spacingLen := toolMarkupWhitespaceLikeLenAt(raw, idx); spacingLen > 0 {
					break
				}
				if xmlTagEndDelimiterLenAt(raw, idx) > 0 || toolMarkupEqualsLenAt(raw, idx) > 0 {
					break
				}
				if _, ok := consumeToolMarkupPipe(raw, idx); ok {
					break
				}
				if _, ok := consumeToolMarkupClosingSlash(raw, idx); ok {
					break
				}
				_, size := utf8.DecodeRuneInString(raw[idx:])
				if size <= 0 {
					idx++
				} else {
					idx += size
				}
			}
			value = raw[valueStart:idx]
		}

		out = append(out, canonicalToolMarkupAttr{
			Key:   key,
			Value: value,
		})
	}
	return out
}

func normalizeCanonicalToolAttrKey(raw string) string {
	trimmed := strings.TrimSpace(removeToolMarkupIgnorables(raw))
	if trimmed == "" {
		return ""
	}
	for _, key := range []string{"name", "call_id", "id", "version"} {
		if next, ok := consumeToolKeyword(trimmed, 0, key); ok && skipToolMarkupIgnorables(trimmed, next) == len(trimmed) {
			return key
		}
	}
	return ""
}

func quoteCanonicalXMLAttrValue(raw string) string {
	if raw == "" {
		return ""
	}
	return strings.ReplaceAll(raw, `"`, "&quot;")
}
