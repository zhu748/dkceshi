package toolcall

import "strings"

// Nested compatibility XML uses ordinary element names, not just tool tags.
// Check it before recursive decoders and every fallback that may re-enter them.
// CDATA and complete JSON strings remain literal and consume no XML depth.
func parameterMarkupLimitExceeded(text string) bool {
	depth, nodes := 0, 0
	for i := 0; i < len(text); {
		next, advanced, blocked := skipXMLIgnoredSection(text, i)
		if blocked {
			return false
		}
		if advanced {
			i = next
			continue
		}
		if text[i] != '<' {
			i++
			continue
		}
		start := i + 1
		closing := start < len(text) && text[start] == '/'
		if closing {
			start++
		}
		if start >= len(text) || !xmlParameterNameStart(text[start]) {
			i++
			continue
		}
		end := findXMLTagEnd(text, start)
		if end < 0 {
			return false
		}
		nodes++
		if closing {
			depth = max(0, depth-1)
		} else if !strings.HasSuffix(strings.TrimSpace(text[i:end]), "/") {
			depth++
		}
		if depth > 128 || nodes > 32768 {
			return true
		}
		i = end + 1
	}
	return false
}

func xmlParameterNameStart(ch byte) bool {
	// This is a conservative budget, not name validation. Count non-ASCII
	// starters as well so Unicode XML elements cannot bypass the depth limit.
	return ch >= 0x80 || ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch == '_' || ch == ':'
}
