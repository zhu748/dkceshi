package sse

import "strings"

// Match only the terminator emitted by prompt.renderMessages: a fixed prefix,
// sixteen hexadecimal digits and a closing bracket. A partial match must wait
// for the next delta; ordinary bracketed text must pass through unchanged.
func scanTranscriptBoundary(tail string) (matched, partial bool) {
	const prefix = "[EndMessage:"
	if tail == "" || tail[0] != '[' {
		return false, false
	}
	if len(tail) < len(prefix) {
		return false, strings.EqualFold(tail, prefix[:len(tail)])
	}
	if !strings.EqualFold(tail[:len(prefix)], prefix) {
		return false, false
	}
	for i := len(prefix); i < len(prefix)+16; i++ {
		if i == len(tail) {
			return false, true
		}
		ch := tail[i]
		hexDigit := ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f' || ch >= 'A' && ch <= 'F'
		if !hexDigit {
			return false, false
		}
	}
	end := len(prefix) + 16
	if len(tail) == end {
		return false, true
	}
	return tail[end] == ']', false
}
