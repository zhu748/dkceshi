package sse

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strconv"
	"time"
)

// ErrUnexpectedStreamEOF distinguishes a transport EOF from a completed turn.
var ErrUnexpectedStreamEOF = errors.New("upstream stream ended without a completion marker")

var errCollectIdle = errors.New("upstream stream idle timeout")

// CollectOptions bounds one non-streaming upstream attempt. Explicit options
// also let callers use short deadlines without changing process-wide settings.
type CollectOptions struct {
	Context              context.Context
	IdleTimeout          time.Duration
	MaxDuration          time.Duration
	TerminalDrainTimeout time.Duration
}

func defaultCollectOptions(resp *http.Response) CollectOptions {
	ctx := context.Background()
	if resp != nil && resp.Request != nil {
		ctx = resp.Request.Context()
	}
	return CollectOptions{
		Context:              ctx,
		IdleTimeout:          collectDuration("DS2API_NONSTREAM_IDLE_TIMEOUT_SECONDS", 300, time.Second),
		MaxDuration:          collectDuration("DS2API_NONSTREAM_MAX_DURATION_SECONDS", 1800, time.Second),
		TerminalDrainTimeout: collectDuration("DS2API_SSE_TERMINAL_DRAIN_MS", 250, time.Millisecond),
	}
}

func collectDuration(key string, fallback int, unit time.Duration) time.Duration {
	n, err := strconv.Atoi(os.Getenv(key))
	if err != nil || n <= 0 || n > 86400 {
		n = fallback
	}
	return time.Duration(n) * unit
}

type collectLimits struct {
	ctx       context.Context
	cancel    context.CancelCauseFunc
	idle      *time.Timer
	drain     *time.Timer
	idleWait  time.Duration
	drainWait time.Duration
	cleanup   func()
}

func newCollectLimits(resp *http.Response, opts CollectOptions) *collectLimits {
	defaults := defaultCollectOptions(resp)
	if opts.Context == nil {
		opts.Context = defaults.Context
	}
	if opts.IdleTimeout <= 0 {
		opts.IdleTimeout = defaults.IdleTimeout
	}
	if opts.MaxDuration <= 0 {
		opts.MaxDuration = defaults.MaxDuration
	}
	if opts.TerminalDrainTimeout <= 0 {
		opts.TerminalDrainTimeout = defaults.TerminalDrainTimeout
	}
	totalCtx, totalCancel := context.WithTimeout(opts.Context, opts.MaxDuration)
	ctx, cancel := context.WithCancelCause(totalCtx)
	l := &collectLimits{ctx: ctx, cancel: cancel, idleWait: opts.IdleTimeout, drainWait: opts.TerminalDrainTimeout}
	l.idle = time.AfterFunc(opts.IdleTimeout, func() { cancel(errCollectIdle) })
	closeCleanup := closeStreamBodyOnCancel(ctx, resp.Body)
	l.cleanup = func() {
		l.idle.Stop()
		if l.drain != nil {
			l.drain.Stop()
		}
		closeCleanup()
		cancel(nil)
		totalCancel()
	}
	return l
}

func (l *collectLimits) activity() { l.idle.Reset(l.idleWait) }

func (l *collectLimits) finish() {
	if l.drain == nil {
		l.drain = time.AfterFunc(l.drainWait, func() { l.cancel(context.DeadlineExceeded) })
	}
}
