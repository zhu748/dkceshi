//go:build !386 && !arm && !mips && !mipsle && !wasm

package util

import (
	"strings"
	"testing"
)

func BenchmarkTokenizerCount(b *testing.B) {
	for _, sample := range []struct{ name, text string }{
		{"context", strings.Repeat("source code and 中文 context.\n", 4096)},
		{"literal_special_tokens", strings.Repeat("<|endoftext|> 中文🙂 ", 256)},
	} {
		b.Run(sample.name, func(b *testing.B) {
			want := countWithTokenizer(sample.text, defaultTokenizerModel)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if got := countWithTokenizer(sample.text, defaultTokenizerModel); got != want {
					b.Fatalf("token count changed: got=%d want=%d", got, want)
				}
			}
		})
	}
}
