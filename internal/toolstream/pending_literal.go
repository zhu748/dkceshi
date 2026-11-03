package toolstream

import (
	"strings"

	"ds2api/internal/toolcall"
)

// A scheduling hint for a held display literal, not a parser. Only new input is
// scanned while its boundary is absent. Full parsing resumes at a possible
// close (including invalid JSON controls), and EOF always examines all input.
type pendingLiteral struct {
	mode             byte
	escaped          bool
	tail             [4]byte
	tailLen          int
	preview          string
	previewAccounted int
	previewBlocked   bool
}

func newPendingLiteral(raw string) *pendingLiteral {
	p := &pendingLiteral{}
	start := 0
	switch {
	case strings.HasPrefix(raw, "\""):
		p.mode, start = 'q', 1
		p.preview = raw
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
	if p.mode != 'q' {
		return p.feedDelimited(text)
	}
	for i := 0; i < len(text); i++ {
		ch := text[i]
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
	}
	return false
}

// Search each chunk in bulk and inspect only the four-byte overlap needed by
// a split Unicode CDATA closer. The fixed buffer never retains the chunk or
// allocates one temporary string per body byte.
func (p *pendingLiteral) feedDelimited(text string) bool {
	var overlap [8]byte
	n := copy(overlap[:], p.tail[:p.tailLen])
	n += copy(overlap[n:], text[:min(4, len(text))])
	boundary := string(overlap[:n])
	closes := []string{"-->"}
	if p.mode == 'd' {
		closes = []string{"]]>", "]]＞", "]]﹥", "]]〉"}
	}
	for _, close := range closes {
		if strings.Contains(text, close) || strings.Contains(boundary, close) {
			return true
		}
	}
	if len(text) >= len(p.tail) {
		p.tailLen = copy(p.tail[:], text[len(text)-len(p.tail):])
	} else {
		p.tailLen = min(len(p.tail), n)
		copy(p.tail[:], overlap[n-p.tailLen:n])
	}
	return false
}

func (s *State) appendPendingText(chunk string) bool {
	if chunk != "" {
		chunk = s.normalizeIncoming(chunk)
		s.pending.WriteString(chunk)
		if s.literalWait != nil {
			if s.literalWait.mode == 'q' && !s.literalWait.previewBlocked {
				s.literalWait.preview += chunk
			}
			if !s.literalWait.feed(chunk) {
				return true
			}
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
