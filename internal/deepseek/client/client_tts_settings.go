package client

import (
	"context"
	"ds2api/internal/auth"
	"ds2api/internal/config"
	dsprotocol "ds2api/internal/deepseek/protocol"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

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
			lastMessage = err.Error()
			attempts++
			if !retryAfterNetworkError(ctx, attempts, err) {
				break
			}
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
			if a.UseConfigToken && c.Auth != nil && c.Auth.SwitchAccount(ctx, a) {
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
			if !retryAfterNetworkError(ctx, attempts, err) {
				break
			}
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
			if a.UseConfigToken && c.Auth != nil && c.Auth.SwitchAccount(ctx, a) {
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
