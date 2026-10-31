package client

import (
	"context"
	"ds2api/internal/auth"
	"ds2api/internal/config"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// SynthesizeSpeech reads an existing assistant message through the TTS
// WebSocket and returns its bare Opus packets.
//
// The caller must pass a session/message pair that belongs to the request's
// account: message ids are account-scoped. The voice lock is held only until
// the ready event arrives, because that event fixes the voice for the stream.
func (c *Client) SynthesizeSpeech(ctx context.Context, a *auth.RequestAuth, sessionID string, messageID int, opts TTSOptions) (*TTSAudio, error) {
	if c == nil || a == nil {
		return nil, errors.New("missing client or auth")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || messageID <= 0 {
		return nil, errors.New("missing session_id or message_id")
	}
	format := strings.TrimSpace(opts.Format)
	if format == "" {
		format = "opus"
	}
	if format != "opus" {
		return nil, fmt.Errorf("unsupported tts format %q", format)
	}
	voice := strings.TrimSpace(opts.Voice)
	if voice != "" && !IsValidTTSVoice(voice) {
		return nil, fmt.Errorf("unknown tts voice %q", voice)
	}
	if opts.ReadTimeout <= 0 {
		opts.ReadTimeout = 120 * time.Second
	}

	// Message ids are account-scoped: the stream must read the message from the
	// same account that produced the recitation. Pin the account for the whole
	// call so the shared auth recovery, which otherwise switches accounts on a
	// 401, cannot hand the read to a different account and turn a recoverable
	// failure into a service_error. Token refresh is unaffected because it keeps
	// the same account id.
	restoreAccount := pinTTSAccount(a)
	defer restoreAccount()
	accountID := a.AccountID
	lock := c.ttsVoiceLockFor(accountID)

	// A long recitation may still be committing when the stream opens; the
	// upstream then answers with finish code 3 (service_error). Retry with a
	// fresh ticket and a short backoff before surfacing the failure.
	const maxAttempts = 3
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Hold the account voice lock only when switching voices, and only until
		// ready fixes the voice for this stream. The audio drain runs outside it,
		// so concurrent TTS calls on the same account are not serialized for the
		// whole audio length.
		hasVoiceLock := false
		if voice != "" {
			lock.Lock()
			hasVoiceLock = true
			if cached, ok := c.cachedTTSVoice(accountID); !ok || cached != voice {
				if err := c.SetTTSVoice(ctx, a, voice, 0); err != nil {
					lock.Unlock()
					return nil, err
				}
				c.rememberTTSVoice(accountID, voice)
			}
		}
		ticket, err := c.GetTTSTicket(ctx, a, 0)
		if err != nil {
			if hasVoiceLock {
				lock.Unlock()
			}
			return nil, err
		}
		ws, resp, err := c.dialTTSWebSocket(ctx, a, sessionID, messageID, ticket, format)
		if err != nil {
			if hasVoiceLock {
				lock.Unlock()
			}
			if resp != nil && resp.Body != nil {
				if err := resp.Body.Close(); err != nil {
					slog.Warn("cleanup close failed", "error", err)
				}
			}
			return nil, err
		}

		// Close the stream as soon as the caller cancels: this covers both
		// waitTTSReady and readTTSAudio so an early failure on another segment
		// immediately aborts the read and frees the account voice lock.
		done := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				if err := ws.Close(); err != nil {
					slog.Warn("cleanup close failed", "error", err)
				}
			case <-done:
			}
		}()

		ready, pending, err := waitTTSReady(ws, opts.ReadTimeout)
		if hasVoiceLock {
			lock.Unlock()
			hasVoiceLock = false
		}
		if err != nil {
			close(done)
			if err := ws.Close(); err != nil {
				slog.Warn("cleanup close failed", "error", err)
			}
			lastErr = err
			var serviceErr *TTSServiceError
			if errors.As(err, &serviceErr) && attempt < maxAttempts {
				config.Logger.Warn("[tts] upstream service error during ready, retrying",
					"account", a.AccountID, "session_id", sessionID, "message_id", messageID,
					"attempt", attempt, "code", serviceErr.Code, "msg", serviceErr.Msg)
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(time.Duration(attempt) * 500 * time.Millisecond):
					continue
				}
			}
			return nil, err
		}
		if voice != "" && ready.VoiceID != "" && ready.VoiceID != voice {
			close(done)
			if err := ws.Close(); err != nil {
				slog.Warn("cleanup close failed", "error", err)
			}
			// The account voice was changed behind our back (another request or
			// the web client). Drop this stream, forget the cached value so the
			// next attempt re-applies the voice, and retry instead of failing.
			c.forgetTTSVoice(accountID)
			lastErr = fmt.Errorf("tts voice mismatch: requested %q, upstream reported %q", voice, ready.VoiceID)
			config.Logger.Warn("[tts] voice mismatch, re-applying voice",
				"account", accountID, "requested", voice, "reported", ready.VoiceID, "attempt", attempt)
			if attempt < maxAttempts {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(time.Duration(attempt) * 200 * time.Millisecond):
					continue
				}
			}
			return nil, lastErr
		}
		audio, err := readTTSAudio(ws, ready, pending, opts)
		close(done)
		if err := ws.Close(); err != nil {
			slog.Warn("cleanup close failed", "error", err)
		}
		if err == nil {
			return audio, nil
		}
		lastErr = err
		var serviceErr *TTSServiceError
		if !errors.As(err, &serviceErr) || attempt == maxAttempts {
			break
		}
		config.Logger.Warn("[tts] upstream service error, retrying",
			"account", a.AccountID, "session_id", sessionID, "message_id", messageID,
			"attempt", attempt, "code", serviceErr.Code, "msg", serviceErr.Msg)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt) * 500 * time.Millisecond):
		}
	}
	return nil, lastErr
}
