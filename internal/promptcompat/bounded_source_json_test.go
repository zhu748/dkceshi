package promptcompat

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSourceJSONBudgetMatchesCanonicalEncodingAtEveryLimit(t *testing.T) {
	for _, value := range []any{
		nil, true, 1.25, json.Number("9007199254740993"), "plain", "\x00\n\r\t\b\f\"\\<>&中文\u2028\u2029\xff",
		map[string]any{"<key>": []any{"value", false, nil, map[string]any{"nested": "<&>"}}},
		[]any{nil, ""}, map[string]any(nil), []any(nil),
	} {
		want, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		for limit := 0; limit <= len(want)+1; limit++ {
			got, fits, err := marshalSourceField(value, limit)
			if err != nil || fits != (len(want) <= limit) || fits && string(got) != string(want) {
				t.Fatalf("limit=%d want=%s got=%s fits=%v error=%v", limit, want, got, fits, err)
			}
		}
	}
}

func TestLargeSourceFieldIsRejectedBeforeEncoding(t *testing.T) {
	value := map[string]any{"file_data": strings.Repeat("x", 16<<20)}
	if data, fits, err := marshalSourceField(value, maxInputSourceEncodedBytes); err != nil || fits || data != nil {
		t.Fatalf("oversized source allocated encoded data: bytes=%d fits=%v error=%v", len(data), fits, err)
	}
}

func TestTypedNullContainersAreRejectedBeforeEncoding(t *testing.T) {
	value := []any{map[string]any(nil), []any(nil)}
	data, _ := json.Marshal(value)
	if got, fits, err := marshalSourceField(value, len(data)-1); err != nil || fits || got != nil {
		t.Fatalf("typed null containers bypassed preflight: bytes=%d fits=%v error=%v", len(got), fits, err)
	}
}

func BenchmarkOversizedSourceFieldPreflight(b *testing.B) {
	value := map[string]any{"file_data": strings.Repeat("x", 16<<20)}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if data, fits, err := marshalSourceField(value, maxInputSourceEncodedBytes); data != nil || fits || err != nil {
			b.Fatal("unexpected encoding")
		}
	}
}
