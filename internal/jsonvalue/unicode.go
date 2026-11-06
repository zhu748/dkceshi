package jsonvalue

import (
	"errors"
	"fmt"
	"unicode/utf8"
)

var ErrInvalidUnicode = errors.New("invalid Unicode in JSON string")

// CheckStringEncoding runs on syntactically valid JSON before encoding/json
// can replace invalid UTF-8 or an unpaired UTF-16 escape with U+FFFD.
func CheckStringEncoding(raw []byte) error {
	if !utf8.Valid(raw) {
		return ErrInvalidUnicode
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] != '"' {
			continue
		}
		for i++; i < len(raw) && raw[i] != '"'; i++ {
			if raw[i] != '\\' {
				continue
			}
			i++
			if i >= len(raw) || raw[i] != 'u' {
				continue
			}
			code := unicodeEscape(raw, i+1)
			i += 4
			if code >= 0xDC00 && code <= 0xDFFF {
				return fmt.Errorf("%w at byte %d", ErrInvalidUnicode, i-5)
			}
			if code < 0xD800 || code > 0xDBFF {
				continue
			}
			if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
				return fmt.Errorf("%w at byte %d", ErrInvalidUnicode, i-5)
			}
			low := unicodeEscape(raw, i+3)
			if low < 0xDC00 || low > 0xDFFF {
				return fmt.Errorf("%w at byte %d", ErrInvalidUnicode, i-5)
			}
			i += 6
		}
	}
	return nil
}

func unicodeEscape(raw []byte, start int) int {
	if start+4 > len(raw) {
		return -1
	}
	value := 0
	for _, ch := range raw[start : start+4] {
		value <<= 4
		switch {
		case ch >= '0' && ch <= '9':
			value += int(ch - '0')
		case ch >= 'a' && ch <= 'f':
			value += int(ch-'a') + 10
		case ch >= 'A' && ch <= 'F':
			value += int(ch-'A') + 10
		default:
			return -1
		}
	}
	return value
}
