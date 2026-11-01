package toolstream

import (
	"unicode/utf8"

	"ds2api/internal/toolcall"
)

// captureProgress scans only newly appended bytes for a possible structural
// close. It is a scheduling hint, not a parser: the existing fence/CDATA-aware
// parser remains authoritative. Literal closes may cause an extra parse, never
// early execution. Long bodies without a close no longer get rescanned for
// every SSE fragment.
type captureProgress struct {
	offset   int
	tagStart int
	literal  toolcall.LiteralState
}

func (p *captureProgress) ready(text string) bool {
	first := p.offset == 0
	ready := first
	for p.offset < len(text) {
		if !utf8.FullRuneInString(text[p.offset:]) {
			break
		}
		r, size := utf8.DecodeRuneInString(text[p.offset:])
		pos := p.offset
		p.offset += size
		if !p.literal.Feed(r) {
			p.tagStart = 0
			continue
		}
		switch r {
		case '<', '＜', '﹤', '〈':
			p.tagStart = pos + 1 // zero means there is no pending tag
		case '>', '＞', '﹥', '〉', '|', '｜', '！', '、':
			if p.tagStart == 0 {
				continue
			}
			terminator := r == '>' || r == '＞' || r == '﹥' || r == '〉'
			if p.offset-p.tagStart > 256 {
				// An oversized or malformed tag is only a hint at its final
				// terminator; pipes in a long CDATA value must not cause scans.
				if terminator {
					ready, p.tagStart = true, 0
				}
				continue
			}
			tagText := text[p.tagStart-1 : p.offset]
			if tag, ok := toolcall.FindToolMarkupTagOutsideIgnored(tagText, 0); ok && tag.Closing {
				ready = true
			}
			if tag, ok := toolcall.FindDSMLTagOutsideIgnored(tagText, 0); ok && tag.Closing {
				ready = true
			}
			if terminator {
				p.tagStart = 0
			}
		}
	}
	return ready
}
