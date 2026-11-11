package client

import (
	"bytes"
	"context"
	"fmt"
	"testing"
)

func BenchmarkUploadReuseLargeFile(b *testing.B) {
	for _, size := range []int{1 << 20, 16 << 20} {
		b.Run(fmt.Sprint(size>>20)+"MiB", func(b *testing.B) {
			c, upstream, a, req := newReuseTestClient()
			req.Filename, req.ContentType = "large.txt", "text/plain"
			req.Data = bytes.Repeat([]byte("x"), size)
			if _, err := c.UploadFilePending(context.Background(), a, req, 1); err != nil {
				b.Fatal(err)
			}
			b.SetBytes(int64(size))
			b.ReportAllocs()
			for b.Loop() {
				file, err := c.UploadFilePending(context.Background(), a, req, 1)
				if err != nil || file.ID != "file-1" {
					b.Fatal(file, err)
				}
			}
			if upstream.uploads != 1 || upstream.fetches != b.N {
				b.Fatal("reuse skipped validation or repeated upload")
			}
		})
	}
}
