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
	} {
		var value any
		if err := Decode(strings.NewReader(raw), &value); !errors.Is(err, ErrDuplicateKey) {
			t.Fatalf("duplicate accepted: %s, error=%v", raw, err)
		}
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
