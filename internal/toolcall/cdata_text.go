package toolcall

import (
	"html"
	"strings"
)

// Decode adjacent literal sections without an XML decoder normalizing line
// endings or rejecting otherwise valid tool strings containing control bytes.
// Real XML children are left to the structured parser; legacy nested examples
// that are not a valid sequence keep the existing literal interpretation.
func parseMixedCDATAString(raw string) (string, bool) {
	var out strings.Builder
	seen := false
	for pos := 0; pos < len(raw); {
		if open := toolCDATAOpenLenAt(raw, pos); open > 0 {
			end := indexToolCDATAClose(raw, pos+open)
			if end < 0 {
				return "", false
			}
			out.WriteString(raw[pos+open : end])
			pos = end + toolCDATACloseLenAt(raw, end)
			seen = true
			continue
		}
		end := strings.IndexByte(raw[pos:], '<')
		if end == 0 {
			return "", false
		}
		if end < 0 {
			end = len(raw) - pos
		}
		out.WriteString(html.UnescapeString(raw[pos : pos+end]))
		pos += end
	}
	return out.String(), seen
}
