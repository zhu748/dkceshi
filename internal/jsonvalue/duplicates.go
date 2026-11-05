package jsonvalue

import (
	"encoding/json"
	"errors"
	"fmt"
)

var ErrDuplicateKey = errors.New("duplicate JSON object key")

// CheckDuplicateKeys checks syntactically valid JSON without decoding values a
// second time. Object keys are decoded so escaped aliases cannot hide a duplicate.
// The iterative stack shares encoding/json's input depth limit.
func CheckDuplicateKeys(raw []byte) error {
	type scope struct {
		keys map[string]struct{}
		key  bool
	}
	var stack []scope
	for i := 0; i < len(raw); i++ {
		switch raw[i] {
		case '{':
			stack = append(stack, scope{keys: map[string]struct{}{}, key: true})
		case '[':
			stack = append(stack, scope{})
		case '}', ']':
			if len(stack) == 0 {
				return nil
			}
			stack = stack[:len(stack)-1]
		case ':':
			if len(stack) > 0 {
				stack[len(stack)-1].key = false
			}
		case ',':
			if len(stack) > 0 && stack[len(stack)-1].keys != nil {
				stack[len(stack)-1].key = true
			}
		case '"':
			start := i
			for i++; i < len(raw); i++ {
				if raw[i] == '\\' {
					i++
					continue
				}
				if raw[i] == '"' {
					break
				}
			}
			if i >= len(raw) {
				return nil
			} // Syntax validation belongs to encoding/json.
			if len(stack) == 0 || !stack[len(stack)-1].key {
				continue
			}
			current := &stack[len(stack)-1]
			var key string
			if err := json.Unmarshal(raw[start:i+1], &key); err != nil {
				return err
			}
			if _, exists := current.keys[key]; exists {
				// Values and names may contain secrets; errors report only position.
				return fmt.Errorf("%w at byte %d", ErrDuplicateKey, start)
			}
			current.keys[key] = struct{}{}
		}
	}
	return nil
}
