package sse

import (
	"context"
	"strings"
	"testing"
)

func BenchmarkParsedLinePump(b *testing.B) {
	body := strings.Repeat("data: {\"v\":\"an independent text fragment\"}\n", 512)
	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	for b.Loop() {
		results, done := startParsedLinePumpWithConfig(context.Background(), strings.NewReader(body), false, "text", AccumulateConfig{})
		count := 0
		for result := range results {
			count += len(result.Parts)
		}
		if err := <-done; err != nil {
			b.Fatal(err)
		}
		if count != 512 {
			b.Fatalf("lost fragments: %d", count)
		}
	}
}
