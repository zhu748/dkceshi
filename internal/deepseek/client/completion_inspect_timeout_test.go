package client

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

func TestCompletionInspectionBoundsFirstByteAndPartialJSON(t *testing.T) {
	for _, prefix := range []string{"", `{"code":`} {
		t.Run(prefix, func(t *testing.T) {
			reader, writer := io.Pipe()
			defer func() {
				if err := writer.Close(); err != nil {
					t.Error(err)
				}
			}()
			written := make(chan error, 1)
			if prefix != "" {
				go func() { _, err := io.WriteString(writer, prefix); written <- err }()
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			_, _, err := inspectCompletionJSONContext(ctx, reader)
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("stalled inspection escaped its deadline: %v", err)
			}
			if prefix != "" {
				if err := <-written; err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
