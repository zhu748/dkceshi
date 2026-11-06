package toolcall

import "strings"

// XML identity, comments and mixed-content order have no lossless mapping in
// the compatibility object decoder. Keep the full source for schema validation
// and regeneration instead of executing a reduced object. Scan only once.
func parameterXMLNeedsLiteral(text string) bool {
	type frame struct{ child, text bool }
	stack := []frame{{}}
	markText := func(value string) bool {
		current := &stack[len(stack)-1]
		current.text = current.text || strings.TrimSpace(value) != ""
		return current.child && current.text
	}
	for i := 0; i < len(text); {
		next, advanced, blocked := skipXMLIgnoredSection(text, i)
		if blocked {
			return false
		}
		if advanced {
			if toolCDATAOpenLenAt(text, i) == 0 {
				return true // Comments and processing instructions carry source data.
			}
			if markText(text[i:next]) {
				return true
			}
			i = next
			continue
		}
		if text[i] != '<' {
			end := strings.IndexByte(text[i:], '<')
			if end < 0 {
				end = len(text) - i
			}
			if markText(text[i : i+end]) {
				return true
			}
			i += end
			continue
		}
		start := i + 1
		closing := start < len(text) && text[start] == '/'
		if closing {
			start++
		}
		if start >= len(text) || !xmlParameterNameStart(text[start]) {
			if markText(text[i : i+1]) {
				return true
			}
			i++
			continue
		}
		end := findXMLTagEnd(text, start)
		if end < 0 {
			return false
		}
		body := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(text[start:end]), "/"))
		nameEnd := strings.IndexAny(body, " \t\r\n")
		if nameEnd >= 0 {
			return true
		}
		if strings.Contains(body, ":") {
			return true
		}
		if closing {
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		} else {
			current := &stack[len(stack)-1]
			current.child = true
			if current.text {
				return true
			}
			if !strings.HasSuffix(strings.TrimSpace(text[start:end]), "/") {
				stack = append(stack, frame{})
			}
		}
		i = end + 1
	}
	return false
}
