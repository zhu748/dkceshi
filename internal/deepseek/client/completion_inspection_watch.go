package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	"ds2api/internal/streambudget"
)

var errInspectionIdle = errors.New("upstream response inspection idle timeout")

type inspectionBody struct {
	io.ReadCloser
	idle   *time.Timer
	active atomic.Bool
	once   sync.Once
	err    error
}

func (b *inspectionBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 && b.active.Load() {
		b.idle.Reset(time.Duration(streambudget.Value("idle_timeout_ms")) * time.Millisecond)
	}
	return n, err
}

func (b *inspectionBody) Close() error {
	b.once.Do(func() { b.err = b.ReadCloser.Close() })
	return b.err
}

// Inspection runs before the SSE consumer owns the response. Its blocked
// reads therefore need their own idle/total budget and cancellation cleanup.
func (c *Client) inspectCompletionFailureContext(parent context.Context, a *auth.RequestAuth, resp *http.Response, source string) error {
	if resp.Body == nil {
		return c.inspectCompletionFailure(a, resp, source)
	}
	total, cancelTotal := context.WithTimeout(parent, time.Duration(streambudget.Value("max_duration_ms"))*time.Millisecond)
	ctx, cancel := context.WithCancelCause(total)
	body := &inspectionBody{ReadCloser: resp.Body}
	body.idle = time.AfterFunc(time.Duration(streambudget.Value("idle_timeout_ms"))*time.Millisecond, func() { cancel(errInspectionIdle) })
	body.active.Store(true)
	closeBody := func() {
		if err := body.Close(); err != nil {
			config.Logger.Warn("[completion_inspection] close failed", "error", err)
		}
	}
	stopClose := context.AfterFunc(ctx, closeBody)
	resp.Body = body
	err := c.inspectCompletionFailure(a, resp, source)
	body.active.Store(false)
	body.idle.Stop()
	if cause := context.Cause(ctx); cause != nil {
		closeBody()
		err = cause
	}
	stopClose()
	cancel(nil)
	cancelTotal()
	return err
}
