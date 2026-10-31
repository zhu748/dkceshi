package audio

import (
	"context"
	"net/http"
	"sync"

	"ds2api/internal/assistantturn"
	"ds2api/internal/auth"
	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
)

type segmentOutcome struct {
	frames    [][]byte
	text      string
	voiceID   string
	audioID   string
	sessionID string
	auth      *auth.RequestAuth
}

func (h *Handler) runSpeechWorkers(runCtx context.Context, cancel context.CancelFunc, workersAuth []*auth.RequestAuth, plan speechPlan) ([]segmentOutcome, *assistantturn.OutputError) {
	segments, model, voice, perAccount := plan.segments, plan.model, plan.voice, plan.perAccount
	gate := newAccountGate(perAccount)

	outcomes := make([]segmentOutcome, len(segments))
	var wg sync.WaitGroup
	var failOnce sync.Once
	var failedOutErr *assistantturn.OutputError
	var failedErr error
	fail := func(err error, outErr *assistantturn.OutputError) {
		failOnce.Do(func() {
			failedErr = err
			failedOutErr = outErr
			cancel()
		})
	}

	process := func(index int, a *auth.RequestAuth) {
		release, err := gate.acquire(runCtx, a.AccountID)
		if err != nil {
			fail(err, nil)
			return
		}
		defer release()
		ctx := auth.WithAuth(runCtx, a)
		recited, outErr := h.recite(ctx, a, model, segments[index])
		outcomes[index].auth = cleanupAuth(a)
		outcomes[index].sessionID = recited.SessionID
		if outErr != nil {
			fail(nil, outErr)
			return
		}
		audio, err := h.DS.SynthesizeSpeech(ctx, a, recited.SessionID, recited.MessageID, dsclient.TTSOptions{
			Voice:  voice,
			Format: "opus",
		})
		outcomes[index].auth = cleanupAuth(a)
		if err != nil {
			config.Logger.Warn("[tts] synthesize failed",
				"account", a.AccountID, "session_id", recited.SessionID, "message_id", recited.MessageID,
				"segment", index+1, "segments", len(segments), "error", err)
			fail(err, nil)
			return
		}
		outcomes[index].frames = audio.Frames
		outcomes[index].text = recited.Text
		outcomes[index].voiceID = audio.VoiceID
		outcomes[index].audioID = audio.AudioID
	}

	segmentQueue := make(chan int, len(segments))
	for i := range segments {
		segmentQueue <- i
	}
	close(segmentQueue)
	for _, workerAuth := range workersAuth {
		wg.Add(1)
		go func(a *auth.RequestAuth) {
			defer wg.Done()
			for {
				select {
				case <-runCtx.Done():
					return
				case index, ok := <-segmentQueue:
					if !ok {
						return
					}
					process(index, a)
				}
			}
		}(workerAuth)
	}
	wg.Wait()
	if failedOutErr != nil {
		return outcomes, failedOutErr
	}
	if failedErr != nil {
		status := http.StatusBadGateway
		if auth.IsNoIdleAccountError(failedErr) {
			status = http.StatusTooManyRequests
		}
		return outcomes, &assistantturn.OutputError{Status: status, Message: failedErr.Error()}
	}

	return outcomes, nil
}
