package textclean

import "strings"

// Large incomplete units accumulate without copying or reparsing their entire
// prefix on every chunk. This schedules a parse only; the full display parser
// still decides whether the completed source is a literal or removable wire.
type pendingScan struct {
	held strings.Builder
	mode byte
	json jsonBoundary
}

func newPendingScan(raw string) *pendingScan {
	if len(raw) <= 4096 {
		return nil
	}
	p := &pendingScan{}
	switch {
	case raw[0] == '"':
		p.mode = 'j'
		p.json.feed(raw)
	case raw[0] == '[' && strings.HasPrefix(strings.TrimSpace(raw[1:]), "{"):
		p.mode = 'j'
		p.json.feed(raw)
	case raw[0] == '`':
		if strings.HasSuffix(raw, "`") {
			return nil
		}
		p.mode = '`'
	case raw[0] == '<':
		p.mode = '>'
		if end := displayTagEnd(raw); end > 0 && compactTag(raw[:end]) == "<|tool|>" {
			tail := strings.TrimSpace(raw[end:])
			if strings.HasPrefix(tail, "{") {
				p.mode = 'j'
				p.json.feed(tail)
			}
		}
	default:
		return nil
	}
	return p
}

func (p *pendingScan) append(text string) bool {
	p.held.WriteString(text)
	if p.mode == 'j' {
		return p.json.feed(text)
	}
	return strings.IndexByte(text, p.mode) >= 0
}

// Only structural completion matters here; JSON syntax/types are checked once
// by jsonValueEnd/isWireToolArray. Escaped quotes cannot trigger a full rescan.
type jsonBoundary struct {
	depth           int
	quoted, escaped bool
}

func (p *jsonBoundary) feed(text string) bool {
	for i := 0; i < len(text); i++ {
		ch := text[i]
		if p.quoted {
			if p.escaped {
				p.escaped = false
				continue
			}
			if ch == '\\' {
				p.escaped = true
				continue
			}
			if ch == '"' {
				p.quoted = false
				if p.depth == 0 {
					return true
				}
			}
			if ch < ' ' {
				return true
			}
			continue
		}
		switch ch {
		case '"':
			p.quoted = true
		case '[', '{':
			p.depth++
		case ']', '}':
			p.depth--
			if p.depth <= 0 {
				return true
			}
		}
	}
	return false
}
