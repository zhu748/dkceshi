package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"

	"ds2api/internal/config"
	"ds2api/internal/streambudget"
)

type autoContinueBody struct {
	*io.PipeReader
	cancel   context.CancelFunc
	mu       sync.Mutex
	upstream *continueUpstreamBody
	closed   bool
}

func (b *autoContinueBody) Close() error {
	b.cancel()
	pipeErr := b.PipeReader.Close()
	b.mu.Lock()
	b.closed = true
	upstream := b.upstream
	b.mu.Unlock()
	if upstream != nil {
		return errors.Join(pipeErr, upstream.Close())
	}
	return pipeErr
}

// Register before the pump can read. Close is synchronous for the current
// body; a continuation arriving after Close is closed before it can be read.
func (b *autoContinueBody) track(body io.ReadCloser) io.ReadCloser {
	if body == nil {
		return nil
	}
	upstream := &continueUpstreamBody{ReadCloser: body}
	b.mu.Lock()
	closed := b.closed
	if !closed {
		b.upstream = upstream
	}
	b.mu.Unlock()
	if closed {
		if err := upstream.Close(); err != nil {
			config.Logger.Warn("[auto_continue] close late upstream body failed", "error", err)
		}
	}
	return upstream
}

type continueUpstreamBody struct {
	io.ReadCloser
	once sync.Once
	err  error
}

func (b *continueUpstreamBody) Close() error {
	b.once.Do(func() { b.err = b.ReadCloser.Close() })
	return b.err
}

// Closing the returned pipe must also interrupt a blocked upstream read.
func newAutoContinueBody(ctx context.Context, initial io.ReadCloser, sessionID string, maxRounds int, openContinue continueOpenFunc) io.ReadCloser {
	if initial == nil || sessionID == "" || openContinue == nil {
		return initial
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if maxRounds <= 0 {
		maxRounds = streambudget.Value("auto_continue_max_rounds")
	}
	pumpCtx, cancel := context.WithCancel(ctx)
	pr, pw := io.Pipe()
	body := &autoContinueBody{PipeReader: pr, cancel: cancel}
	initial = body.track(initial)
	open := func(ctx context.Context, session string, message int) (*http.Response, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		response, err := openContinue(ctx, session, message)
		if response != nil {
			response.Body = body.track(response.Body)
		}
		return response, err
	}
	go func() {
		defer cancel()
		pumpAutoContinue(pumpCtx, pw, initial, continueState{sessionID: sessionID}, maxRounds, open)
	}()
	return body
}

func readContinueRound(ctx context.Context, pw *io.PipeWriter, body io.ReadCloser, state *continueState) (bool, error) {
	var once sync.Once
	closeBody := func() {
		once.Do(func() {
			if err := body.Close(); err != nil {
				config.Logger.Warn("[auto_continue] close upstream body failed", "error", err)
			}
		})
	}
	stop := context.AfterFunc(ctx, closeBody)
	defer func() { closeBody(); stop() }()
	return streamBodyWithContinueState(ctx, pw, body, state)
}
