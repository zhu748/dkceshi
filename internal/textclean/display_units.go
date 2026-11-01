package textclean

import (
	"encoding/json"
	"strings"

	"ds2api/internal/toolcall"
)

func cleanDisplayUnit(raw string, final bool) (end int, text string, wait bool) {
	for _, literal := range []struct{ open, close string }{{"<!--", "-->"}, {"<![CDATA[", "]]>"}} {
		if strings.HasPrefix(literal.open, raw) && !final {
			return 0, "", true
		}
		if strings.HasPrefix(raw, literal.open) {
			end := strings.Index(raw[len(literal.open):], literal.close)
			if end < 0 {
				if !final {
					return 0, "", true
				}
				return len(raw), raw, false
			}
			end += len(literal.open) + len(literal.close)
			return end, raw[:end], false
		}
	}
	if raw[0] == '"' {
		for i := 1; i < len(raw); i++ {
			if raw[i] == '\\' {
				i++
				continue
			}
			if raw[i] < ' ' {
				return 1, raw[:1], false
			}
			if raw[i] == '"' {
				return i + 1, raw[:i+1], false
			}
		}
		return len(raw), raw, !final
	}
	if raw[0] == '[' {
		tail := strings.TrimLeft(raw[1:], " \t\r\n")
		if tail == "" {
			return len(raw), raw, !final
		}
		if tail[0] == '{' {
			end, incomplete := jsonValueEnd(raw)
			if incomplete && !final {
				return 0, "", true
			}
			if end > 0 {
				if isWireToolArray(raw[:end]) {
					return end, "", false
				}
				return end, raw[:end], false
			}
		}
	}
	if raw[0] != '<' {
		return 0, "", false
	}
	end = displayTagEnd(raw)
	if end == 0 {
		return len(raw), raw, !final
	}
	tag := compactTag(raw[:end])
	if tag == "<think>" || isAgentOpen(tag) {
		closeStart, closeEnd := displayWrapperClose(raw, end, tag)
		if closeStart < 0 && !final {
			return 0, "", true
		}
		if closeStart < 0 && tag == "<think>" {
			return len(raw), "", false
		}
		body, consumed := raw[end:], len(raw)
		if closeStart >= 0 {
			body, consumed = raw[end:closeStart], closeEnd
		}
		if isAgentOpen(tag) {
			body = strings.TrimPrefix(body, "<result>")
			if closeStart >= 0 {
				body = strings.TrimSuffix(body, "</result>")
			}
		}
		return consumed, CleanVisibleOutput(body, false), false
	}
	if tag == "</result>" {
		tail := strings.TrimLeft(raw[end:], " \t\r\n")
		if tail == "" && !final {
			return 0, "", true
		}
		if strings.HasPrefix(tail, "<") {
			n := displayTagEnd(tail)
			if n == 0 && !final {
				return 0, "", true
			}
			if n > 0 && isAgentClose(compactTag(tail[:n])) {
				return len(raw) - len(tail) + n, "", false
			}
		}
	}
	if isAgentClose(tag) {
		return end, "", false
	}
	if tag == "<|tool|>" {
		tail := strings.TrimLeft(raw[end:], " \t\r\n")
		if tail == "" && !final {
			return 0, "", true
		}
		if strings.HasPrefix(tail, "{") {
			n, incomplete := jsonValueEnd(tail)
			if incomplete && !final {
				return 0, "", true
			}
			var obj map[string]any
			if n > 0 && json.Unmarshal([]byte(tail[:n]), &obj) == nil {
				if id, _ := obj["tool_call_id"].(string); strings.HasPrefix(id, "call") {
					return len(raw) - len(tail) + n, "", false
				}
			}
		}
	}
	if open, ok := toolcall.FindToolMarkupTagOutsideIgnored(raw, 0); ok && open.Start == 0 {
		if open.Closing && open.EPSELike {
			return open.End + 1, "", false
		}
		if !open.Closing && open.Name == "tool_calls" {
			if close, ok := toolcall.FindMatchingToolMarkupClose(raw, open); ok {
				return close.End + 1, "", false
			}
			if !final {
				return 0, "", true
			}
		}
	}
	return end, cleanControlToken(raw[:end]), false
}

func compactTag(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.ReplaceAll(text, "｜", "|")), ""))
}

func isAgentOpen(tag string) bool {
	return tag == "<attempt_completion>" || tag == "<ask_followup_question>" || tag == "<new_task>"
}

func isAgentClose(tag string) bool { return strings.HasPrefix(tag, "</") && isAgentOpen("<"+tag[2:]) }

func displayTagEnd(text string) int {
	var quote byte
	for i := 1; i < len(text); i++ {
		ch := text[i]
		if quote != 0 {
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			continue
		}
		if ch == '>' {
			return i + 1
		}
	}
	return 0
}
