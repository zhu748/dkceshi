package toolcall

import "strings"

// Recover only a suffix of closing containers after a complete compound value.
// CDATA can swallow an array close, and nested question/options arrays may also
// omit enclosing ]}] at EOF. Existing bytes and values are never rewritten.
// The caller must validate the entire candidate against the declared schema.
func parseArrayMissingClosures(raw string) (any, bool) {
	text := strings.TrimSpace(raw)
	if !strings.HasPrefix(text, "[") || (!strings.HasSuffix(text, "}") && !strings.HasSuffix(text, "]")) {
		return nil, false
	}
	closes := make([]byte, 0, 8)
	inString, escaped := false, false
	for i := 0; i < len(text); i++ {
		ch := text[i]
		if inString {
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '[', '{':
			if len(closes) >= 128 {
				return nil, false
			}
			close := byte(']')
			if ch == '{' {
				close = '}'
			}
			closes = append(closes, close)
		case ']', '}':
			if len(closes) == 0 || closes[len(closes)-1] != ch {
				return nil, false
			}
			closes = closes[:len(closes)-1]
		}
	}
	if inString || len(closes) == 0 {
		return nil, false
	}
	for left, right := 0, len(closes)-1; left < right; left, right = left+1, right-1 {
		closes[left], closes[right] = closes[right], closes[left]
	}
	// Full JSON decoding rejects missing values, commas, extra roots and other
	// syntax damage. Only the suffix dictated by the existing stack is tried.
	return parseJSONLiteralValue(text + string(closes))
}
