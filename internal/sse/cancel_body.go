package sse

import (
	"context"
	"io"
	"log/slog"
	"sync"
)

// The returned cleanup must run before the pump exits. A cancelled pump can
// win the race against its AfterFunc and stop that callback before it closes
// the blocked reader, so cancellation also performs the close synchronously.
func closeStreamBodyOnCancel(ctx context.Context, body io.Reader) func() {
	closer, ok := body.(io.Closer)
	if !ok {
		return func() {}
	}
	var once sync.Once
	closeBody := func() {
		once.Do(func() {
			if err := closer.Close(); err != nil {
				slog.Warn("[sse] close cancelled stream failed", "error", err)
			}
		})
	}
	stop := context.AfterFunc(ctx, closeBody)
	return func() {
		if ctx.Err() != nil {
			closeBody()
		}
		stop()
	}
}
