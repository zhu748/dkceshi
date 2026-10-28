package client

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	dsprotocol "ds2api/internal/deepseek/protocol"
	trans "ds2api/internal/deepseek/transport"
)

// TTS voice ids exposed by the DeepSeek web client. They are account-level
// server-side settings; the WebSocket query carries no voice parameter.
const (
	TTSVoiceMira   = "mira"
	TTSVoiceEcho   = "echo"
	TTSVoiceStella = "stella"
	TTSVoiceTide   = "tide"
)

var ttsVoiceIDs = map[string]struct{}{
	TTSVoiceMira:   {},
	TTSVoiceEcho:   {},
	TTSVoiceStella: {},
	TTSVoiceTide:   {},
}

// TTSVoices returns the supported voice ids in the order the web client shows
// them.
func TTSVoices() []string {
	return []string{TTSVoiceMira, TTSVoiceEcho, TTSVoiceStella, TTSVoiceTide}
}

// IsValidTTSVoice reports whether voiceID is one of the four upstream voices.
func IsValidTTSVoice(voiceID string) bool {
	_, ok := ttsVoiceIDs[strings.TrimSpace(voiceID)]
	return ok
}

const (
	maxTTSAudioBytes = 64 << 20
	maxTTSFrames     = 200000
)

// TTSOptions controls one speech synthesis request.
type TTSOptions struct {
	// Voice switches the account voice before the stream starts. Empty keeps
	// the account's current voice (the server default is mira).
	Voice string
	// Format is the upstream audio format. Only "opus" is currently known.
	Format string
	// ReadTimeout bounds each individual WebSocket read.
	ReadTimeout time.Duration
}

// TTSAudio is the raw upstream result: a sequence of bare Opus packets plus
// the metadata echoed by the ready event.
type TTSAudio struct {
	Frames  [][]byte
	AudioID string
	VoiceID string
	Format  string
	TraceID string
}

type ttsEvent struct {
	Event   string `json:"event"`
	AudioID string `json:"audio_id"`
	Format  string `json:"format"`
	VoiceID string `json:"voice_id"`
	TraceID string `json:"trace_id"`
	Code    int    `json:"code"`
	Msg     string `json:"msg"`
}

// GetTTSTicket exchanges the account token for a short-lived WebSocket ticket.
// The endpoint uses Bearer auth and does not require PoW.
func (c *Client) GetTTSTicket(ctx context.Context, a *auth.RequestAuth, maxAttempts int) (string, error) {
	if c == nil || a == nil {
		return "", errors.New("missing client or auth")
	}
	if maxAttempts <= 0 {
		maxAttempts = c.maxRetries
	}
	attempts := 0
	lastMessage := ""
	for attempts < maxAttempts {
		clients := c.requestClientsForAuth(ctx, a)
		headers := c.authHeadersForAuth(a)
		resp, status, err := c.postJSONWithStatus(ctx, clients.regular, clients.fallback, dsprotocol.DeepSeekAuthTicketURL, headers, map[string]any{"scope": "tts"})
		if err != nil {
			config.Logger.Warn("[tts_ticket] request error", "error", err, "account", a.AccountID)
			attempts++
			continue
		}
		code, bizCode, msg, bizMsg := extractResponseStatus(resp)
		if status == http.StatusOK && code == 0 && bizCode == 0 {
			data, _ := resp["data"].(map[string]any)
			bizData, _ := data["biz_data"].(map[string]any)
			ticket, _ := bizData["ticket"].(string)
			if strings.TrimSpace(ticket) != "" {
				return strings.TrimSpace(ticket), nil
			}
			lastMessage = "missing tts ticket in response"
		} else {
			lastMessage = failureMessage(msg, bizMsg, "get tts ticket failed")
			config.Logger.Warn("[tts_ticket] failed", "status", status, "code", code, "biz_code", bizCode, "msg", msg, "biz_msg", bizMsg, "account", a.AccountID)
		}
		if ch := DetectCaptchaChallenge(resp); ch != nil {
			config.Logger.Warn("[tts_ticket] captcha challenge detected", "account", a.AccountID, "instruction", ch.Instruction)
			c.coolDownAfterCaptcha(a, "tts_ticket")
			if a.UseConfigToken && c.Auth.SwitchAccount(ctx, a) {
				attempts++
				continue
			}
		}
		if c.recoverManagedAuth(ctx, a, &attempts, status, code, bizCode, msg, bizMsg) {
			continue
		}
		attempts++
	}
	if lastMessage == "" {
		lastMessage = "get tts ticket failed"
	}
	return "", errors.New(lastMessage)
}

// SetTTSVoice persists the account-level voice selection. The upstream applies
// it immediately: the next TTS WebSocket ready event reports the new voice.
func (c *Client) SetTTSVoice(ctx context.Context, a *auth.RequestAuth, voiceID string, maxAttempts int) error {
	if c == nil || a == nil {
		return errors.New("missing client or auth")
	}
	voiceID = strings.TrimSpace(voiceID)
	if !IsValidTTSVoice(voiceID) {
		return fmt.Errorf("unknown tts voice %q", voiceID)
	}
	if maxAttempts <= 0 {
		maxAttempts = c.maxRetries
	}
	attempts := 0
	lastMessage := ""
	for attempts < maxAttempts {
		clients := c.requestClientsForAuth(ctx, a)
		headers := c.authHeadersForAuth(a)
		resp, status, err := c.postJSONWithStatus(ctx, clients.regular, clients.fallback, dsprotocol.DeepSeekTTSSetVoiceURL, headers, map[string]any{"voice_id": voiceID})
		if err != nil {
			config.Logger.Warn("[tts_voice] request error", "error", err, "account", a.AccountID, "voice", voiceID)
			attempts++
			continue
		}
		code, bizCode, msg, bizMsg := extractResponseStatus(resp)
		if status == http.StatusOK && code == 0 && bizCode == 0 {
			return nil
		}
		lastMessage = failureMessage(msg, bizMsg, "set tts voice failed")
		config.Logger.Warn("[tts_voice] failed", "status", status, "code", code, "biz_code", bizCode, "msg", msg, "biz_msg", bizMsg, "account", a.AccountID, "voice", voiceID)
		if ch := DetectCaptchaChallenge(resp); ch != nil {
			config.Logger.Warn("[tts_voice] captcha challenge detected", "account", a.AccountID, "instruction", ch.Instruction)
			c.coolDownAfterCaptcha(a, "tts_voice")
			if a.UseConfigToken && c.Auth.SwitchAccount(ctx, a) {
				attempts++
				continue
			}
		}
		if c.recoverManagedAuth(ctx, a, &attempts, status, code, bizCode, msg, bizMsg) {
			continue
		}
		attempts++
	}
	if lastMessage == "" {
		lastMessage = "set tts voice failed"
	}
	return errors.New(lastMessage)
}

// TTSServiceError is the upstream's explicit failure event. It is retryable:
// the message may not have been committed yet when the stream opens, and a
// fresh ticket plus a short wait usually succeeds.
type TTSServiceError struct {
	Code int
	Msg  string
}

func (e *TTSServiceError) Error() string {
	return fmt.Sprintf("tts service error: code=%d msg=%s", e.Code, e.Msg)
}

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

func (c *Client) dialTTSWebSocket(ctx context.Context, a *auth.RequestAuth, sessionID string, messageID int, ticket string, format string) (*trans.WSConn, *http.Response, error) {
	pathQuery := dsprotocol.DeepSeekTTSTargetPath +
		"?chat_session_id=" + url.QueryEscape(sessionID) +
		"&message_id=" + strconv.Itoa(messageID) +
		"&ticket=" + url.QueryEscape(ticket) +
		"&mode=manual" +
		"&format=" + url.QueryEscape(format)

	var dial trans.DialContextFunc
	if proxyCfg, ok := c.resolveProxyForAccount(a.Account); ok {
		var err error
		dial, err = proxyDialContext(proxyCfg)
		if err != nil {
			return nil, nil, err
		}
	} else if a.Account.NoProxy {
		dial = (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	}
	return trans.DialWebSocket(ctx, trans.WebSocketDialOptions{
		Host:           dsprotocol.DeepSeekHost,
		PathQuery:      pathQuery,
		DialContext:    dial,
		Origin:         "https://chat.deepseek.com",
		UserAgent:      c.authHeadersForAuth(a)["User-Agent"],
		AcceptLanguage: dsprotocol.AcceptLanguageFor(a.Account.Locale),
	})
}

func (c *Client) ttsVoiceLockFor(accountID string) *sync.Mutex {
	key := strings.TrimSpace(accountID)
	if key == "" {
		key = "_"
	}
	c.ttsVoiceLocksMu.Lock()
	defer c.ttsVoiceLocksMu.Unlock()
	if c.ttsVoiceLocks == nil {
		c.ttsVoiceLocks = map[string]*sync.Mutex{}
	}
	lock := c.ttsVoiceLocks[key]
	if lock == nil {
		lock = &sync.Mutex{}
		c.ttsVoiceLocks[key] = lock
	}
	return lock
}

// pinTTSAccount locks one TTS stream to the account that produced its
// recitation. An explicitly targeted account is left alone. The returned
// function restores the previous state.
func pinTTSAccount(a *auth.RequestAuth) func() {
	if a == nil || strings.TrimSpace(a.AccountID) == "" || strings.TrimSpace(a.TargetAccount) != "" {
		return func() {}
	}
	a.TargetAccount = a.AccountID
	return func() { a.TargetAccount = "" }
}

// cachedTTSVoice returns the last voice this process set on the account.
func (c *Client) cachedTTSVoice(accountID string) (string, bool) {
	if c == nil {
		return "", false
	}
	c.ttsVoiceLocksMu.Lock()
	defer c.ttsVoiceLocksMu.Unlock()
	voice, ok := c.ttsVoiceKnown[accountID]
	return voice, ok
}

func (c *Client) rememberTTSVoice(accountID, voice string) {
	if c == nil || accountID == "" {
		return
	}
	c.ttsVoiceLocksMu.Lock()
	defer c.ttsVoiceLocksMu.Unlock()
	if c.ttsVoiceKnown == nil {
		c.ttsVoiceKnown = map[string]string{}
	}
	c.ttsVoiceKnown[accountID] = voice
}

func (c *Client) forgetTTSVoice(accountID string) {
	if c == nil || accountID == "" {
		return
	}
	c.ttsVoiceLocksMu.Lock()
	defer c.ttsVoiceLocksMu.Unlock()
	delete(c.ttsVoiceKnown, accountID)
}

// waitTTSReady consumes frames until the ready event arrives. Binary frames
// that race ahead of it are returned for the audio loop.
func waitTTSReady(ws *trans.WSConn, timeout time.Duration) (ttsEvent, [][]byte, error) {
	var pending [][]byte
	pendingBytes := 0
	for {
		if err := ws.SetReadDeadline(time.Now().Add(timeout)); err != nil {
			return ttsEvent{}, nil, err
		}
		opcode, payload, err := ws.ReadMessage()
		if err != nil {
			return ttsEvent{}, nil, fmt.Errorf("tts ready: %w", err)
		}
		switch opcode {
		case trans.WSOpBinary:
			pendingBytes += len(payload)
			if len(pending) >= maxTTSFrames || pendingBytes > maxTTSAudioBytes {
				return ttsEvent{}, nil, errors.New("tts pending audio frames exceed limit")
			}
			pending = append(pending, payload)
		case trans.WSOpText:
			var ev ttsEvent
			if err := json.Unmarshal(payload, &ev); err != nil {
				config.Logger.Warn("[tts] unexpected text frame before ready", "payload", string(payload), "error", err)
				continue
			}
			switch ev.Event {
			case "ready":
				return ev, pending, nil
			case "error":
				return ttsEvent{}, nil, &TTSServiceError{Code: ev.Code, Msg: ev.Msg}
			}
		}
	}
}

// readTTSAudio drains the binary frames, waits for the server finish event and
// acknowledges it. Sending the client finish earlier truncates the audio.
func readTTSAudio(ws *trans.WSConn, ready ttsEvent, pending [][]byte, opts TTSOptions) (*TTSAudio, error) {
	audio := &TTSAudio{
		AudioID: ready.AudioID,
		VoiceID: ready.VoiceID,
		Format:  ready.Format,
		TraceID: ready.TraceID,
	}
	if audio.Format == "" {
		audio.Format = opts.Format
	}
	if audio.Format == "" {
		audio.Format = "opus"
	}

	totalBytes := 0
	expectedSeq := uint32(0)
	sequenceGaps := 0
	addFrame := func(payload []byte) error {
		if len(payload) < 4 {
			return errors.New("tts binary frame too short")
		}
		seq := binary.BigEndian.Uint32(payload[:4])
		data := payload[4:]
		if len(data) == 0 {
			return nil
		}
		if len(audio.Frames) >= maxTTSFrames {
			return fmt.Errorf("tts returned more than %d frames", maxTTSFrames)
		}
		totalBytes += len(data)
		if totalBytes > maxTTSAudioBytes {
			return errors.New("tts audio exceeds size limit")
		}
		// Keep every frame in arrival order. The sequence number is metadata:
		// long streams repeat or skip it (one Opus packet can carry several
		// frames), and deduplicating by sequence silently dropped audio from
		// the middle of the reply.
		if seq != expectedSeq {
			sequenceGaps++
		}
		expectedSeq = seq + 1
		audio.Frames = append(audio.Frames, append([]byte(nil), data...))
		return nil
	}
	for _, payload := range pending {
		if err := addFrame(payload); err != nil {
			return nil, err
		}
	}

	finished := false
	for !finished {
		if err := ws.SetReadDeadline(time.Now().Add(opts.ReadTimeout)); err != nil {
			return nil, err
		}
		opcode, payload, err := ws.ReadMessage()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("tts stream: %w", err)
		}
		switch opcode {
		case trans.WSOpBinary:
			if err := addFrame(payload); err != nil {
				return nil, err
			}
		case trans.WSOpText:
			var ev ttsEvent
			if err := json.Unmarshal(payload, &ev); err != nil {
				continue
			}
			switch ev.Event {
			case "finish":
				if ev.Code != 0 {
					return nil, &TTSServiceError{Code: ev.Code, Msg: ev.Msg}
				}
				finished = true
			case "error":
				return nil, fmt.Errorf("tts upstream error: code=%d msg=%s", ev.Code, ev.Msg)
			}
		}
	}
	if !finished {
		return nil, errors.New("tts stream ended before finish event")
	}
	if err := ws.WriteText([]byte(`{"event":"finish"}`)); err != nil {
		config.Logger.Warn("[tts] acknowledge finish failed", "error", err)
	}
	if sequenceGaps > 0 {
		config.Logger.Warn("[tts] non-contiguous frame sequence",
			"gaps", sequenceGaps, "frames", len(audio.Frames))
	}
	if len(audio.Frames) == 0 {
		return nil, errors.New("tts returned no audio")
	}
	return audio, nil
}
