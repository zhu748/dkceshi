package jsonvalue

import (
	"crypto/sha256"
	"encoding/json"
	"strings"
	"testing"
)

func TestJSONFingerprintMatchesTypedMessageEncoding(t *testing.T) {
	for _, value := range []any{
		[]map[string]any(nil), []map[string]any{},
		[]map[string]any{nil, {"role": "user", "content": "\x00<&>\u2028\ufffd", "metadata": map[string]any{"n": json.Number("9007199254740993")}}},
	} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		size, sum, err := JSONFingerprint(value)
		if err != nil || size != len(encoded) || sum != sha256.Sum256(encoded) {
			t.Fatalf("fingerprint changed canonical JSON: %T: %v", value, err)
		}
	}
}

func BenchmarkMessageBoundaryFingerprint(b *testing.B) {
	messages := []map[string]any{{"role": "user", "content": strings.Repeat("<>&中文\n", 400000)}}
	b.Run("encoded", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			data, err := json.Marshal(messages)
			if err != nil {
				b.Fatal(err)
			}
			_ = sha256.Sum256(data)
		}
	})
	b.Run("incremental", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, _, err := JSONFingerprint(messages); err != nil {
				b.Fatal(err)
			}
		}
	})
}
