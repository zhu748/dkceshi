package toolcall

import "strings"

// FindUnfinishedToolLiteralStart holds an incomplete display literal until the
// scanner can distinguish a quoted example from tool protocol. This is used
// only by display guards; tool argument parsing retains its own CDATA policy.
func FindUnfinishedToolLiteralStart(text string) int {
	for i := 0; i < len(text); {
		if text[i] == '<' && (strings.HasPrefix("<![CDATA[", strings.ToUpper(text[i:])) || strings.HasPrefix("<!--", text[i:])) {
			return i
		}
		next, advanced, blocked := skipXMLIgnoredSection(text, i)
		if blocked {
			return i
		}
		if advanced {
			i = next
			continue
		}
		if end, ok := markdownCodeSpanEnd(text, i); ok {
			i = end
			continue
		}
		if text[i] == '"' {
			unfinished := true
			for end := i + 1; end < len(text); end++ {
				if text[end] == '\\' {
					end++
				} else if text[end] == '"' || text[end] < ' ' {
					unfinished = false
					break
				}
			}
			if unfinished {
				return i
			}
		}
		i++
	}
	return -1
}
