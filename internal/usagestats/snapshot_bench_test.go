package usagestats

import (
	"fmt"
	"testing"
)

func benchmarkEntries(n int) []Entry {
	entries := make([]Entry, n)
	for i := range entries {
		entries[i] = Entry{
			Date: fmt.Sprintf("2026-10-%02d", 1+i%30), Model: "deepseek-v4.1-flash",
			CallerID: fmt.Sprintf("caller:%08d", i), AccountID: "account:0123456789abcdef",
			AccountLabel: "Test account", LastUsedAt: 1793448000000,
			PromptTokens: 1000, CompletionTokens: 400, ReasoningTokens: 200, TotalTokens: 1400,
			Cost: 0.125, Calls: 10,
		}
	}
	return entries
}

func BenchmarkWriteSnapshot(b *testing.B) {
	for _, n := range []int{10000, 100000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			entries := benchmarkEntries(n)
			payload := fileData{Version: FileVersion, UpdatedAt: 1793448000000, Entries: make([]Entry, n)}
			var size int
			write := func(_ string, body []byte) error { size = len(body); return nil }
			b.ReportAllocs()
			for b.Loop() {
				copy(payload.Entries, entries)
				if err := writeSnapshot("unused", payload, write); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(size), "file-bytes/op")
		})
	}
}
