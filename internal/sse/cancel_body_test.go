package sse

import (
	"context"
	"io"
	"sync/atomic"
	"testing"
)

type closeCountingBody struct{ calls atomic.Int32 }

func (b *closeCountingBody) Read([]byte) (int, error) { return 0, io.EOF }
func (b *closeCountingBody) Close() error {
	b.calls.Add(1)
	return nil
}

func TestCancelledPumpCleanupClosesBeforeReturningExactlyOnce(t *testing.T) {
	for i := 0; i < 200; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		body := &closeCountingBody{}
		cleanup := closeStreamBodyOnCancel(ctx, body)
		cancel()
		cleanup()
		cleanup()
		if got := body.calls.Load(); got != 1 {
			t.Fatalf("cancelled reader not closed exactly once before return: %d", got)
		}
	}
}

func TestFinishedPumpDetachesCloseFromLaterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	body := &closeCountingBody{}
	closeStreamBodyOnCancel(ctx, body)()
	cancel()
	if got := body.calls.Load(); got != 0 {
		t.Fatalf("completed pump unexpectedly closed its caller-owned body: %d", got)
	}
}
