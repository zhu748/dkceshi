package sse

import "strings"

const ToolRoleMarker = "[Tool]:"

// ToolRoleMarkerGuard cuts invented tool-result turns while preserving literal
// markers in CDATA, quoted strings and Markdown code across arbitrary chunks.
// Only a short unfinished delimiter is held; the complete response is not rescanned.
type ToolRoleMarkerGuard struct {
	pending      string
	tripped      bool
	inCDATA      bool
	quote        bool
	escaped      bool
	code         byte
	ticks        int
	fence        bool
	fenceClosing bool
	notLineStart bool
}

func (g *ToolRoleMarkerGuard) Tripped() bool           { return g != nil && g.tripped }
func (g *ToolRoleMarkerGuard) Feed(text string) string { return g.feed(text, false) }

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
				if !final && i+n == len(data) {
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
			if !final && i+n == len(data) {
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
			return data[:i]
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
