package jsonvalue

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"hash"
	"sort"
	"unicode/utf8"
)

// Hash decoded JSON with encoding/json's canonical escaping and key order.
// A fixed buffer avoids allocating an encoded copy of large attachment strings.
type contextJSONHash struct {
	hash       hash.Hash
	buffer     [4096]byte
	used, size int
}

var contextJSONSafeASCII = func() [utf8.RuneSelf]bool {
	var safe [utf8.RuneSelf]bool
	for i := 32; i < len(safe); i++ {
		safe[i] = true
	}
	for _, ch := range []byte{'"', '\\', '<', '>', '&'} {
		safe[ch] = false
	}
	return safe
}()

// JSONFingerprint hashes canonical JSON without allocating an encoded copy of
// large decoded strings. Its bytes match encoding/json, including HTML escaping.
func JSONFingerprint(value any) (int, [32]byte, error) {
	w := contextJSONHash{hash: sha256.New()}
	if err := w.value(value, 0); err != nil {
		return 0, [32]byte{}, err
	}
	w.flush()
	var sum [32]byte
	copy(sum[:], w.hash.Sum(nil))
	return w.size, sum, nil
}

func (w *contextJSONHash) flush() {
	// hash.Hash.Write cannot fail or return a short write.
	_, _ = w.hash.Write(w.buffer[:w.used])
	w.used = 0
}

func (w *contextJSONHash) write(text string) {
	w.size += len(text)
	for len(text) > 0 {
		if w.used == len(w.buffer) {
			w.flush()
		}
		n := copy(w.buffer[w.used:], text)
		w.used += n
		text = text[n:]
	}
}

func (w *contextJSONHash) quoted(text string) {
	w.write(`"`)
	start := 0
	for i := 0; i < len(text); {
		if ch := text[i]; ch < utf8.RuneSelf && contextJSONSafeASCII[ch] {
			i++
			continue
		}
		ch, size, escape := text[i], 1, ""
		switch ch {
		case '"', '\\':
			escape = "\\" + text[i:i+1]
		case '\n':
			escape = `\n`
		case '\r':
			escape = `\r`
		case '\t':
			escape = `\t`
		case '\b':
			escape = `\b`
		case '\f':
			escape = `\f`
		default:
			if ch < 32 || ch == '<' || ch == '>' || ch == '&' {
				const digits = "0123456789abcdef"
				escape = `\u00` + string([]byte{digits[ch>>4], digits[ch&15]})
			} else if ch >= utf8.RuneSelf {
				r, n := utf8.DecodeRuneInString(text[i:])
				size = n
				switch {
				case r == utf8.RuneError && n == 1:
					escape = `\ufffd`
				case r == '\u2028':
					escape = `\u2028`
				case r == '\u2029':
					escape = `\u2029`
				}
			}
		}
		if escape != "" {
			w.write(text[start:i])
			w.write(escape)
			start = i + size
		}
		i += size
	}
	w.write(text[start:])
	w.write(`"`)
}

func (w *contextJSONHash) value(value any, depth int) error {
	if depth > 10000 {
		return fmt.Errorf("context JSON exceeds nesting limit")
	}
	switch value := value.(type) {
	case string:
		w.quoted(value)
	case map[string]any:
		if value == nil {
			w.write("null")
			return nil
		}
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		w.write("{")
		for i, key := range keys {
			if i > 0 {
				w.write(",")
			}
			w.quoted(key)
			w.write(":")
			if err := w.value(value[key], depth+1); err != nil {
				return err
			}
		}
		w.write("}")
	case []map[string]any:
		if value == nil {
			w.write("null")
			return nil
		}
		w.write("[")
		for i, item := range value {
			if i > 0 {
				w.write(",")
			}
			if err := w.value(item, depth+1); err != nil {
				return err
			}
		}
		w.write("]")

	case []any:
		if value == nil {
			w.write("null")
			return nil
		}
		w.write("[")
		for i, item := range value {
			if i > 0 {
				w.write(",")
			}
			if err := w.value(item, depth+1); err != nil {
				return err
			}
		}
		w.write("]")
	default:
		// Numbers retain json.Number's precision and validation; typed internal
		// callers and MarshalJSON implementations keep encoding/json semantics.
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		w.write(string(data))
	}
	return nil
}
