package textclean

import "strings"

// Find structural closes in the original coordinate system. Lowercasing the
// whole body changes offsets for Unicode text; substring matching also treats
// quoted/commented/example closes as real structure and ignores nesting.
func displayWrapperClose(text string, from int, open string) (int, int) {
	closeTag := "</" + open[1:]
	depth := 1
	for i := from; i < len(text); {
		if end := displayLiteralEnd(text, i); end > i {
			i = end
			continue
		}
		if text[i] != '<' {
			i++
			continue
		}
		n := displayTagEnd(text[i:])
		if n == 0 {
			break
		}
		tag := compactTag(text[i : i+n])
		if tag == open {
			depth++
		}
		if tag == closeTag {
			depth--
			if depth == 0 {
				return i, i + n
			}
		}
		i += n
	}
	return -1, -1
}

func displayLiteralEnd(text string, start int) int {
	raw := text[start:]
	for _, literal := range []struct{ open, close string }{{"<!--", "-->"}, {"<![CDATA[", "]]>"}} {
		if strings.HasPrefix(raw, literal.open) {
			if end := strings.Index(raw[len(literal.open):], literal.close); end >= 0 {
				return start + len(literal.open) + end + len(literal.close)
			}
			return len(text)
		}
	}
	if raw[0] == '"' {
		for i := 1; i < len(raw); i++ {
			if raw[i] == '\\' {
				i++
				continue
			}
			if raw[i] < ' ' {
				return start
			}
			if raw[i] == '"' {
				return start + i + 1
			}
		}
		return len(text)
	}
	if raw[0] == '`' || raw[0] == '~' {
		run := markerRun(raw, raw[0])
		if run >= 3 && displayLineStart(text, start) {
			return start + displayFenceEnd(raw, run)
		}
		if raw[0] == '`' {
			if end := inlineCodeEnd(raw, run); end > 0 {
				return start + end
			}
			return len(text)
		}
	}
	return start
}

func displayLineStart(text string, i int) bool {
	for i--; i >= 0; i-- {
		if text[i] == '\n' || text[i] == '\r' {
			return true
		}
		if text[i] != ' ' && text[i] != '\t' {
			return false
		}
	}
	return true
}

func displayFenceEnd(text string, run int) int {
	for pos := strings.IndexByte(text, '\n'); pos >= 0 && pos < len(text); {
		pos++
		end := strings.IndexByte(text[pos:], '\n')
		if end < 0 {
			end = len(text)
		} else {
			end += pos
		}
		line := strings.TrimLeft(text[pos:end], " \t")
		n := markerRun(line, text[0])
		if n >= run && strings.TrimSpace(line[n:]) == "" {
			return end
		}
		if end == len(text) {
			break
		}
		pos = end
	}
	return len(text)
}
