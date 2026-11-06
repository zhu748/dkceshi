package jsonvalue

import (
	"errors"
	"testing"
)

func TestJSONRejectsUnpairedSurrogatesWithoutRewriting(t *testing.T) {
	for _, raw := range []string{`"\ud800"`, `"\udc00"`, `{"x":"a\uD800b"}`, `{"\udfff":1}`, `["\ud800\ud800"]`} {
		var value any
		if err := Unmarshal([]byte(raw), &value); !errors.Is(err, ErrInvalidUnicode) {
			t.Fatalf("invalid string accepted: %s, err=%v", raw, err)
		}
	}
	for _, raw := range []string{`"\ud83d\ude00"`, `"😀"`, `"\\ud800"`, `"\uFFFD"`, `{"x":"\"\\\ud83d\ude00"}`} {
		var value any
		if err := Unmarshal([]byte(raw), &value); err != nil {
			t.Fatalf("valid literal rejected: %s: %v", raw, err)
		}
	}
}
