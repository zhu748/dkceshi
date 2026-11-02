package toolstream

import (
	"strings"
	"unicode/utf8"
)

// Preview only ordinary bytes while the authoritative parser retains the
// complete quoted source. Ambiguous markup stays buffered, including at EOF.
// When that source is classified, discard the already displayed prefix once.
func (s *State) previewQuotedText() []Event {
	if s.literalWait == nil || s.literalWait.mode != 'q' {
		return nil
	}
	p := s.literalWait
	if p.previewBlocked {
		return nil
	}
	if n := s.previewEmitted - p.previewAccounted; n > 0 {
		p.preview = p.preview[min(n, len(p.preview)):]
		p.previewAccounted = s.previewEmitted
	}
	raw := p.preview
	end := 0
	for end < len(raw) {
		if !utf8.FullRuneInString(raw[end:]) {
			break
		}
		ch, size := utf8.DecodeRuneInString(raw[end:])
		if strings.ContainsRune("<＜﹤〈[`~", ch) || quoteFilterPrefix(raw[end:]) {
			p.previewBlocked = strings.ContainsRune("<＜﹤〈[`~", ch) || len(raw[end:]) >= len("CONTENT_FILTER")
			break
		}
		end += size
	}
	text := raw[:end]
	p.preview = raw[end:]
	p.previewAccounted += end
	s.previewEmitted += end
	if text == "" {
		return nil
	}
	return []Event{{Content: text, preview: true}}
}

func quoteFilterPrefix(text string) bool {
	const marker = "CONTENT_FILTER"
	if text[0] != 'C' && text[0] != 'c' {
		return false
	}
	n := min(len(text), len(marker))
	return strings.EqualFold(text[:n], marker[:n])
}

func (s *State) consumePreview(text string) string {
	if text != "" && s.previewEmitted > 0 {
		s.previewSkip += s.previewEmitted
		s.previewEmitted = 0
	}
	n := min(s.previewSkip, len(text))
	s.previewSkip -= n
	return text[n:]
}
