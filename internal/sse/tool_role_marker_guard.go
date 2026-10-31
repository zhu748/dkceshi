package sse

import "strings"

const ToolRoleMarker = "[Tool]:"

// ToolRoleMarkerGuard cuts invented tool-result turns and transcript boundaries
// while preserving literal markers in CDATA, quoted strings and Markdown code.
// Only a short unfinished delimiter is held; the complete response is not rescanned.
type ToolRoleMarkerGuard struct {
	pending      string
	tripped      bool
	reason       CutReason
	inCDATA      bool
	quote        bool
	escaped      bool
	code         byte
	ticks        int
	fence        bool
	fenceClosing bool
	notLineStart bool
}

func (g *ToolRoleMarkerGuard) Tripped() bool { return g != nil && g.tripped }

// maxGuardPendingRunLen bounds how long a withheld backtick/tilde run may be.
// The only unbounded withhold in this guard is a run of fence characters that
// reaches the end of the chunk: without a cap, a degenerate upstream streaming
// megabytes of backticks would grow g.pending without limit and re-concatenate
// it on every Feed. Real Markdown fences are a handful of ticks, so a run
// beyond this cap is treated as already-complete input to the run handlers
// below instead of being withheld.
const maxGuardPendingRunLen = 512

func (g *ToolRoleMarkerGuard) Feed(text string) string { return g.feed(text, false) }

func (g *ToolRoleMarkerGuard) Reason() CutReason {
	if g == nil {
		return CutNone
	}
	return g.reason
}

func (g *ToolRoleMarkerGuard) feed(text string, final bool) string {
	if g == nil || g.tripped {
		return ""
	}
	data := g.pending + text
	g.pending = ""
	i := 0
	for i < len(data) {
		tail := data[i:]
		if g.inCDATA {
			if strings.HasPrefix(tail, "]]>") {
				g.inCDATA = false
				i += 3
				continue
			}
			if !final && strings.HasPrefix("]]>", tail) {
				break
			}
			i++
			continue
		}
		if g.code != 0 {
			if g.fenceClosing {
				if data[i] == '\n' || data[i] == '\r' {
					g.code = 0
					g.fenceClosing = false
					g.notLineStart = false
					i++
					continue
				}
				if data[i] == ' ' || data[i] == '\t' {
					i++
					continue
				}
				g.fenceClosing = false
			}
			if data[i] == g.code {
				n := 0
				for i+n < len(data) && data[i+n] == g.code {
					n++
				}
				if !final && i+n == len(data) && n <= maxGuardPendingRunLen {
					break
				}
				if !g.fence && n == g.ticks {
					g.code = 0
					g.ticks = 0
				} else if g.fence && !g.notLineStart && n >= g.ticks {
					g.fenceClosing = true
				}
				i += n
				g.notLineStart = true
				continue
			}
			if data[i] == '\n' || data[i] == '\r' {
				g.notLineStart = false
			} else if data[i] != ' ' && data[i] != '\t' {
				g.notLineStart = true
			}
			i++
			continue
		}
		if g.quote {
			if g.escaped {
				g.escaped = false
			} else if data[i] == '\\' {
				g.escaped = true
			} else if data[i] == '"' {
				g.quote = false
			}
			i++
			continue
		}
		const cdata = "<![CDATA["
		if strings.HasPrefix(tail, cdata) {
			g.inCDATA = true
			i += len(cdata)
			continue
		}
		if !final && strings.HasPrefix(cdata, tail) {
			break
		}
		if data[i] == '`' || (data[i] == '~' && !g.notLineStart) {
			ch := data[i]
			n := 0
			for i+n < len(data) && data[i+n] == ch {
				n++
			}
			if !final && i+n == len(data) && n <= maxGuardPendingRunLen {
				break
			}
			if ch == '`' || n >= 3 {
				g.code = ch
				g.ticks = n
				g.fence = n >= 3 && !g.notLineStart
			}
			i += n
			g.notLineStart = true
			continue
		}
		if data[i] == '"' {
			g.quote = true
			i++
			continue
		}
		if len(tail) >= len(ToolRoleMarker) && strings.EqualFold(tail[:len(ToolRoleMarker)], ToolRoleMarker) {
			g.tripped = true
			g.reason = CutToolRoleMarker
			return data[:i]
		}
		if matched, partial := scanTranscriptBoundary(tail); matched {
			g.tripped = true
			g.reason = CutTranscriptBoundary
			return data[:i]
		} else if partial && !final {
			break
		}
		if !final && len(tail) < len(ToolRoleMarker) && strings.EqualFold(tail, ToolRoleMarker[:len(tail)]) {
			break
		}
		if data[i] == '\n' || data[i] == '\r' {
			g.notLineStart = false
		} else if data[i] != ' ' && data[i] != '\t' {
			g.notLineStart = true
		}
		i++
	}
	g.pending = data[i:]
	return data[:i]
}

func TrimToolRoleMarker(text string) (string, bool) {
	var g ToolRoleMarkerGuard
	kept := g.feed(text, true)
	return kept, g.tripped
}
