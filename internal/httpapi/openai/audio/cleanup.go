package audio

import (
	"context"
	"strings"
	"sync"
	"time"

	"ds2api/internal/auth"
	"ds2api/internal/config"
)

func (h *Handler) deleteSession(ctx context.Context, a *auth.RequestAuth, sessionID string) {
	if sessionID == "" || a == nil || strings.TrimSpace(a.DeepSeekToken) == "" {
		return
	}
	deleteCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if _, err := h.DS.DeleteSessionForToken(deleteCtx, a.DeepSeekToken, sessionID); err != nil {
		config.Logger.Warn("[tts] delete recitation session failed",
			"account", a.AccountID, "session_id", sessionID, "error", err)
	}
}

// cleanupAuth retains only the immutable routing identity needed to delete a
// session. Worker auth changes across retries/segments and owns mutable maps;
// background cleanup must never retain that live request object.
func cleanupAuth(a *auth.RequestAuth) *auth.RequestAuth {
	if a == nil {
		return nil
	}
	return &auth.RequestAuth{
		UseConfigToken: a.UseConfigToken, DeepSeekToken: a.DeepSeekToken,
		CallerID: a.CallerID, UserAgent: a.UserAgent, AccountID: a.AccountID,
		Account: a.Account,
	}
}

// cleanupSpeechSessions starts bounded background cleanup after the response.
// Outcomes already contain snapshots of each session's routing identity.
func (h *Handler) cleanupSpeechSessions(baseCtx context.Context, outcomes []segmentOutcome) {
	var sessionsToDelete []struct {
		auth      *auth.RequestAuth
		sessionID string
	}
	for _, outcome := range outcomes {
		if outcome.sessionID != "" && outcome.auth != nil {
			sessionsToDelete = append(sessionsToDelete, struct {
				auth      *auth.RequestAuth
				sessionID string
			}{auth: outcome.auth, sessionID: outcome.sessionID})
		}
	}
	if len(sessionsToDelete) == 0 {
		return
	}
	go func() {
		cleanCtx, cancelClean := context.WithTimeout(context.WithoutCancel(baseCtx), 15*time.Second)
		defer cancelClean()
		var cleanWg sync.WaitGroup
		cleanSem := make(chan struct{}, 8)
		for _, item := range sessionsToDelete {
			cleanWg.Add(1)
			go func(a *auth.RequestAuth, sID string) {
				defer cleanWg.Done()
				select {
				case cleanSem <- struct{}{}:
					defer func() { <-cleanSem }()
				case <-cleanCtx.Done():
					return
				}
				h.deleteSession(auth.WithAuth(cleanCtx, a), a, sID)
			}(item.auth, item.sessionID)
		}
		cleanWg.Wait()
	}()
}
