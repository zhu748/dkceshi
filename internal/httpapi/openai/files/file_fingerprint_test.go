package files

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"testing"
)

func BenchmarkInlineFileFingerprint(b *testing.B) {
	for _, size := range []int{1 << 20, 8 << 20} {
		data := bytes.Repeat([]byte("x"), size)
		prefix := "application/octet-stream\x00sample.bin\x00"
		expected := fmt.Sprintf("%x", sha256.Sum256(append([]byte(prefix), data...)))
		if got := inlineFileFingerprint("sample.bin", "application/octet-stream", data); got != expected {
			b.Fatal("cache identity changed")
		}
		b.Run(fmt.Sprintf("%dMiB/copy", size>>20), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(size))
			for b.Loop() {
				sum := sha256.Sum256(append([]byte(prefix), data...))
				_ = fmt.Sprintf("%x", sum[:])
			}
		})
		b.Run(fmt.Sprintf("%dMiB/stream", size>>20), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(size))
			for b.Loop() {
				_ = inlineFileFingerprint("sample.bin", "application/octet-stream", data)
			}
		})
	}
}
