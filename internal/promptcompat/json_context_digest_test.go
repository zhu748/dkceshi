package promptcompat

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestStreamingContextDigestMatchesJSONEncoding(t *testing.T) {
	for _, value := range []any{
		nil, true, false, json.Number("9007199254740993"), 1.5, -0.1, map[string]any(nil), []any(nil),
		"\x00\n\r\t\b\f\"\\<>&中文\u2028\u2029\xff", strings.Repeat("<&>中文", 3000),
		map[string]any{"z": []any{nil, json.Number("1e12"), "\xff"}, "a": map[string]any{"nested": "\r\n"}},
		json.RawMessage(` {"raw":"<"} `), []string{"typed", "array"},
	} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		got, err := digestJSONContext(value)
		if err != nil || got != digestContext(encoded) {
			t.Fatalf("hash mismatch for %T: %v", value, err)
		}
	}
	for _, invalid := range []any{math.NaN(), json.Number("invalid"), make(chan int)} {
		if _, err := digestJSONContext(invalid); err == nil {
			t.Fatalf("invalid JSON accepted: %T", invalid)
		}
	}
}

func FuzzStreamingContextDigest(f *testing.F) {
	f.Add(`{"a":[null,9007199254740993,"\\<>&中文"],"b":false}`)
	f.Add(`{"\ufffd":"\u2028\u2029","x":"\u0000"}`)
	f.Fuzz(func(t *testing.T, raw string) {
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err != nil {
			return
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return
		}
		got, err := digestJSONContext(value)
		if err != nil || got != digestContext(encoded) {
			t.Fatal("streaming JSON digest differs from canonical encoding")
		}
	})
}

func BenchmarkLargeInputFieldDigests(b *testing.B) {
	request := map[string]any{"messages": []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_file", "file_data": strings.Repeat("A", 16<<20)}}}}}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = InputFieldDigests(request)
	}
}
