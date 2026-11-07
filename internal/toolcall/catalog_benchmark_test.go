package toolcall

import (
	"fmt"
	"testing"
)

func BenchmarkNormalizeLargeToolCatalog(b *testing.B) {
	for _, withSchema := range []bool{false, true} {
		b.Run(fmt.Sprintf("schema_%t", withSchema), func(b *testing.B) { benchmarkToolCatalog(b, withSchema) })
	}
}

func benchmarkToolCatalog(b *testing.B, withSchema bool) {
	tools := make([]any, 1024)
	schema := map[string]any{"type": "object", "properties": map[string]any{"value": map[string]any{"type": "string"}}}
	for i := range tools {
		function := map[string]any{"name": fmt.Sprintf("tool_%d", i)}
		if withSchema {
			function["parameters"] = schema
		}
		tools[i] = map[string]any{"type": "function", "function": function}
	}
	calls := make([]ParsedToolCall, 128)
	for i := range calls {
		calls[i] = ParsedToolCall{Name: fmt.Sprintf("tool_%d", i), Input: map[string]any{"value": "unchanged"}}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out := NormalizeParsedToolCallsForSchemas(calls, tools)
		if len(out) != len(calls) || out[0].Input["value"] != "unchanged" {
			b.Fatal("normalization changed arguments")
		}
	}
}
