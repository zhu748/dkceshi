package toolcall

// LiteralState is a bounded incremental scheduling aid. It identifies quoted,
// CDATA, comment and Markdown literal bytes across chunks; it never executes or
// rewrites source. The authoritative parser still verifies complete structures.
type LiteralState struct {
	tail           [9]byte
	mode           byte
	quote, escaped bool
	code           rune
	ticks, run     int
	runChar        rune
}

// Feed reports whether r can belong to structural markup rather than a literal.
func (s *LiteralState) Feed(r rune) bool {
	ch := literalASCII(r)
	copy(s.tail[:], s.tail[1:])
	s.tail[len(s.tail)-1] = ch
	if s.mode != 0 {
		if s.mode == 'd' && s.ends("]]>") || s.mode == 'c' && s.ends("-->") {
			s.mode = 0
		}
		return false
	}
	if s.quote {
		if s.escaped {
			s.escaped = false
		} else if r == '\\' {
			s.escaped = true
		} else if r == '"' {
			s.quote = false
		}
		return false
	}
	if s.run > 0 && r != s.runChar {
		if s.code == 0 && (s.runChar == '`' || s.run >= 3) {
			s.code, s.ticks = s.runChar, s.run
		} else if s.code == s.runChar && s.run >= s.ticks {
			s.code, s.ticks = 0, 0
		}
		s.run = 0
	}
	if r == '`' || r == '~' {
		if s.runChar != r {
			s.run = 0
		}
		s.runChar, s.run = r, s.run+1
		return false
	}
	if s.code != 0 {
		return false
	}
	if s.ends("<![cdata[") {
		s.mode = 'd'
		return false
	}
	if s.ends("<!--") {
		s.mode = 'c'
		return false
	}
	if r == '"' {
		s.quote = true
		return false
	}
	return true
}

func (s *LiteralState) ends(text string) bool {
	start := len(s.tail) - len(text)
	for i := range len(text) {
		if s.tail[start+i] != text[i] {
			return false
		}
	}
	return true
}

func literalASCII(r rune) byte {
	switch r {
	case '＜', '﹤', '〈':
		return '<'
	case '＞', '﹥', '〉':
		return '>'
	case '！', '、':
		return '!'
	}
	if r >= 'A' && r <= 'Z' {
		return byte(r + 'a' - 'A')
	}
	if r < 128 {
		return byte(r)
	}
	return 0
}
