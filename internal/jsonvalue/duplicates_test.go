package jsonvalue

import (
	"errors"
	"strings"
	"testing"
)

func TestDuplicateKeysRejectAmbiguityAtEveryDepth(t *testing.T) {
	for _, raw := range []string{
		`{"path":"a","path":"b"}`,
		`{"input":[{"path":"a","\u0070ath":"b"}]}`,
		`{"input":{"nested":{"x":1,"x":2}}}`,
		`{"名":1,"\u540d":2}`,
	} {
		var value any
		if err := Decode(strings.NewReader(raw), &value); !errors.Is(err, ErrDuplicateKey) {
			t.Fatalf("duplicate accepted: %s, error=%v", raw, err)
		}
	}
}

func TestDuplicateKeyDecoderFallbackForInvalidUTF8(t *testing.T) {
	// CheckDuplicateKeys is also called directly before Unicode validation.
	// encoding/json replaces this invalid byte with U+FFFD; both spellings
	// must continue to compare equal even on the plain-key fast path.
	raw := []byte("{\"\xff\":1,\"\ufffd\":2}")
	if err := CheckDuplicateKeys(raw); !errors.Is(err, ErrDuplicateKey) {
		t.Fatalf("invalid UTF-8 alias bypassed duplicate detection: %v", err)
	}
}

func TestSameKeysInSeparateObjectsAndQuotedContentArePreserved(t *testing.T) {
	raw := `{"items":[{"path":"a"},{"path":"b"}],"content":"{\"x\":1,\"x\":2}","large":9007199254740993}`
	var value any
	if err := Unmarshal([]byte(raw), &value); err != nil {
		t.Fatal(err)
	}
	if value.(map[string]any)["content"] != `{"x":1,"x":2}` {
		t.Fatal("quoted content was changed")
	}
}
