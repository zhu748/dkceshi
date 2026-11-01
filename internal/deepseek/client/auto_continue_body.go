package client

import (
	"context"
	"io"
	"sync"

	"ds2api/internal/config"
)

type autoContinueBody struct {
	*io.PipeReader
	cancel context.CancelFunc
}

func (b *autoContinueBody) Close() error {
	b.cancel()
	return b.PipeReader.Close()
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
		maxRounds = defaultAutoContinueLimit
	}
	pumpCtx, cancel := context.WithCancel(ctx)
	pr, pw := io.Pipe()
	go func() {
		defer cancel()
		pumpAutoContinue(pumpCtx, pw, initial, continueState{sessionID: sessionID}, maxRounds, openContinue)
	}()
	return &autoContinueBody{PipeReader: pr, cancel: cancel}
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
