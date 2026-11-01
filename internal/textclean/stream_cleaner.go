package textclean

import "strings"

// StreamCleaner owns display cleanup across chunks. Ordinary prose and fenced
// code stream immediately; ambiguous tokens/inline literals wait for their
// boundary. It never parses, repairs, or emits executable tool calls.
type StreamCleaner struct {
	pending      string
	fence        string
	notLineStart bool
	waiting      *pendingScan
}

func (c *StreamCleaner) Write(text string) string {
	if c.waiting != nil {
		if !c.waiting.append(text) {
			return ""
		}
		c.pending += c.waiting.held.String()
		c.waiting = nil
	} else {
		c.pending += text
	}
	out := c.drain(false)
	if c.fence == "" {
		c.waiting = newPendingScan(c.pending)
	}
	return out
}

func (c *StreamCleaner) Flush() string {
	if c.waiting != nil {
		c.pending += c.waiting.held.String()
		c.waiting = nil
	}
	return c.drain(true)
}

func (c *StreamCleaner) drain(final bool) string {
	var out strings.Builder
	for c.pending != "" {
		if c.fence != "" {
			n := c.fencedPrefix(final)
			if n == 0 {
				break
			}
			c.emit(&out, c.pending[:n], n)
			continue
		}
		raw := c.pending
		if raw[0] == '`' || raw[0] == '~' {
			run := markerRun(raw, raw[0])
			if run == len(raw) && !final {
				break
			}
			if !c.notLineStart && run >= 3 {
				c.fence = raw[:run]
				c.emit(&out, raw[:run], run)
				continue
			}
			if raw[0] == '`' {
				end := inlineCodeEnd(raw, run)
				if end == len(raw) && !final {
					break
				}
				if end == 0 && !final {
					break
				}
				if end == 0 {
					end = len(raw)
				}
				c.emit(&out, raw[:end], end)
				continue
			}
			c.emit(&out, raw[:run], run)
			continue
		}
		end, replacement, wait := cleanDisplayUnit(raw, final)
		if wait {
			break
		}
		if end > 0 {
			c.emit(&out, replacement, end)
			continue
		}
		end = 1
		for end < len(raw) && !strings.ContainsRune("`~<[\"", rune(raw[end])) {
			end++
		}
		c.emit(&out, raw[:end], end)
	}
	return out.String()
}

func (c *StreamCleaner) emit(out *strings.Builder, text string, consumed int) {
	out.WriteString(text)
	// Context belongs to source bytes, even when a control token was removed.
	for _, ch := range c.pending[:consumed] {
		switch ch {
		case '\n', '\r':
			c.notLineStart = false
		case ' ', '\t':
		default:
			c.notLineStart = true
		}
	}
	c.pending = c.pending[consumed:]
}

func (c *StreamCleaner) fencedPrefix(final bool) int {
	raw := c.pending
	if !c.notLineStart {
		trimmed := strings.TrimLeft(raw, " \t")
		run := markerRun(trimmed, c.fence[0])
		if len(trimmed) == 0 || run == len(trimmed) {
			if !final {
				return 0
			}
		}
		if run >= len(c.fence) {
			tail := trimmed[run:]
			if end := strings.IndexByte(tail, '\n'); end >= 0 {
				if strings.TrimSpace(tail[:end]) == "" {
					c.fence = ""
				}
			} else if strings.TrimSpace(tail) == "" {
				if !final {
					return 0
				}
				c.fence = ""
			}
		}
	}
	if end := strings.IndexByte(raw, '\n'); end >= 0 {
		return end + 1
	}
	return len(raw)
}

func markerRun(text string, marker byte) int {
	i := 0
	for i < len(text) && text[i] == marker {
		i++
	}
	return i
}

func inlineCodeEnd(text string, run int) int {
	for i := run; i < len(text); {
		if text[i] != '`' {
			i++
			continue
		}
		n := markerRun(text[i:], '`')
		if n == run {
			return i + n
		}
		i += n
	}
	return 0
}
