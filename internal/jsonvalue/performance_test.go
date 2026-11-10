package jsonvalue

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
)

func BenchmarkJSONDecode(b *testing.B) {
	for _, sample := range []struct{ name, item string }{
		{"numbers", `123,1.25,0,-42,9007199254740993`},
		{"messages", `{"role":"tool","name":"read","tool_call_id":"call_1","content":"ordinary result","metadata":{"success":true}}`},
	} {
		b.Run(sample.name, func(b *testing.B) {
			items := strings.Repeat(sample.item+",", 1024)
			raw := []byte("[" + strings.TrimSuffix(items, ",") + "]")
			b.SetBytes(int64(len(raw)))
			b.ReportAllocs()
			for b.Loop() {
				var value any
				if err := Unmarshal(raw, &value); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestNumberNormalizationPreservesRepresentationBoundaries(t *testing.T) {
	for _, raw := range []string{"0", "-0", "1", "1.25", "1.250", "1e2", "1e+06", "1e-6", "1e-07"} {
		var value any
		if err := Unmarshal([]byte(raw), &value); err != nil {
			t.Fatal(err)
		}
		if _, ok := value.(float64); !ok {
			t.Fatalf("safe number %s changed representation: %T", raw, value)
		}
		if raw == "-0" && !math.Signbit(value.(float64)) {
			t.Fatal("negative zero was lost")
		}
	}
	for _, raw := range []string{"9007199254740993", "123456789012345678901234567890", "0.1234567890123456789", "1.0000000000000001", "1e-400", "1e400", "1e401"} {
		var value any
		if err := Unmarshal([]byte(raw), &value); err != nil {
			t.Fatal(err)
		}
		if number, ok := value.(json.Number); !ok || string(number) != raw {
			t.Fatalf("lossless number %s changed: %T %v", raw, value, value)
		}
	}
}

func BenchmarkDuplicateKeys(b *testing.B) {
	var text strings.Builder
	text.WriteByte('{')
	for i := 0; i < 4096; i++ {
		if i > 0 {
			text.WriteByte(',')
		}
		fmt.Fprintf(&text, `"parameter_%d":null`, i)
	}
	text.WriteByte('}')
	raw := []byte(text.String())
	b.ReportAllocs()
	for b.Loop() {
		if err := CheckDuplicateKeys(raw); err != nil {
			b.Fatal(err)
		}
	}
}
