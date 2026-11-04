package client

import (
	"context"
	"io"
	"time"

	"ds2api/internal/config"
	"ds2api/internal/streambudget"
)

// Response headers can arrive without a first body byte. Inspection happens
// before the SSE consumer starts its idle timer, so it needs its own bounded
// read that closes the network body on timeout or caller cancellation.
func inspectCompletionJSONContext(ctx context.Context, body io.ReadCloser) (io.ReadCloser, map[string]any, error) {
	if body == nil {
		return nil, nil, nil
	}
	bounded, cancel := context.WithTimeout(ctx, time.Duration(streambudget.Value("connect_timeout_ms"))*time.Millisecond)
	done := make(chan struct{})
	stop := context.AfterFunc(bounded, func() {
		defer close(done)
		if err := body.Close(); err != nil {
			config.Logger.Warn("[completion] cancel response inspection failed", "error", err)
		}
	})
	replay, parsed, err := inspectCompletionJSON(body)
	if !stop() {
		<-done
	}
	if bounded.Err() != nil {
		err = bounded.Err()
	}
	cancel()
	return replay, parsed, err
}
