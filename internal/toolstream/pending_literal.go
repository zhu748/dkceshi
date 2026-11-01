package toolstream

import (
	"strings"

	"ds2api/internal/toolcall"
)

// A scheduling hint for a held display literal, not a parser. Only new input is
// scanned while its boundary is absent. Full parsing resumes at a possible
// close (including invalid JSON controls), and EOF always examines all input.
type pendingLiteral struct {
	mode    byte
	escaped bool
	tail    string
}

func newPendingLiteral(raw string) *pendingLiteral {
	p := &pendingLiteral{}
	start := 0
	switch {
	case strings.HasPrefix(raw, "\""):
		p.mode, start = 'q', 1
	case strings.HasPrefix(raw, "<!--"):
		p.mode, start = 'c', 4
	case len(raw) >= 9 && strings.EqualFold(raw[:9], "<![CDATA["):
		p.mode, start = 'd', 9
	default:
		return nil
	}
	if p.feed(raw[start:]) {
		return nil
	}
	return p
}

func (p *pendingLiteral) feed(text string) bool {
	for i := 0; i < len(text); i++ {
		ch := text[i]
		if p.mode == 'q' {
			if p.escaped {
				p.escaped = false
				continue
			}
			if ch == '\\' {
				p.escaped = true
				continue
			}
			if ch == '"' || ch < ' ' {
				return true
			}
			continue
		}
		p.tail += text[i : i+1]
		if len(p.tail) > 6 {
			p.tail = p.tail[len(p.tail)-6:]
		}
		if p.mode == 'c' && strings.HasSuffix(p.tail, "-->") {
			return true
		}
		if p.mode == 'd' {
			for _, close := range []string{"]]>", "]]＞", "]]﹥", "]]〉"} {
				if strings.HasSuffix(p.tail, close) {
					return true
				}
			}
		}
	}
	return false
}

func (s *State) appendPendingText(chunk string) bool {
	if chunk != "" {
		chunk = s.normalizeIncoming(chunk)
		s.pending.WriteString(chunk)
		if s.literalWait != nil && !s.literalWait.feed(chunk) {
			return true
		}
	}
	// An empty chunk explicitly flushes the full parser.
	s.literalWait = nil
	return false
}

func (s *State) holdUnfinishedDisplayLiteral(pending string, start int) (string, bool) {
	if !s.DeferMalformed {
		return "", false
	}
	literal := toolcall.FindUnfinishedToolLiteralStart(pending)
	partial := findPartialXMLToolTagStart(pending)
	if literal < 0 || insideCodeFenceWithState(s, pending[:literal]) || (start >= 0 && literal >= start) || (partial >= 0 && literal >= partial) {
		return "", false
	}
	prefix := pending[:literal]
	s.pending.Reset()
	s.pending.WriteString(pending[literal:])
	s.literalWait = newPendingLiteral(pending[literal:])
	s.noteText(prefix)
	return prefix, true
}
