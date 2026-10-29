package client

import (
	"context"
	dsprotocol "ds2api/internal/deepseek/protocol"
	"errors"
	"fmt"
	"net/http"

	"ds2api/internal/auth"
	"ds2api/internal/config"
)

// DeleteSessionResult 删除会话结果
type DeleteSessionResult struct {
	SessionID    string // 会话 ID
	Success      bool   // 是否成功
	ErrorMessage string // 错误信息
}

// DeleteSession 删除单个会话
func (c *Client) DeleteSession(ctx context.Context, a *auth.RequestAuth, sessionID string, maxAttempts int) (*DeleteSessionResult, error) {
	if maxAttempts <= 0 {
		maxAttempts = c.maxRetries
	}
	result := &DeleteSessionResult{
		SessionID: sessionID,
	}

	if sessionID == "" {
		result.ErrorMessage = "session_id is required"
		return result, errors.New(result.ErrorMessage)
	}

	attempts := 0
	refreshed := false

	for attempts < maxAttempts {
		clients := c.requestClientsForAuth(ctx, a)
		headers := c.authHeadersForAuth(a)

		payload := map[string]any{
			"chat_session_id":  sessionID,
			"chat_session_ids": nil,
		}

		resp, status, err := c.postJSONWithStatus(ctx, clients.regular, clients.fallback, dsprotocol.DeepSeekDeleteSessionURL, headers, payload)
		if err != nil {
			config.Logger.Warn("[delete_session] request error", "error", err, "session_id", sessionID)
			attempts++
			continue
		}

		code, bizCode, msg, bizMsg := extractResponseStatus(resp)
		if deleteResponseError(resp, status) == nil {
			result.Success = true
			c.uploadCache.invalidateOwner(uploadReuseOwner(a.DeepSeekToken))
			return result, nil
		}

		result.ErrorMessage = fmt.Sprintf("status=%d, code=%d, msg=%s", status, code, msg)
		config.Logger.Warn("[delete_session] failed", "status", status, "code", code, "biz_code", bizCode, "msg", msg, "biz_msg", bizMsg, "session_id", sessionID)

		if a.UseConfigToken && c.Auth != nil {
			if isTokenInvalid(status, code, bizCode, msg, bizMsg) && !refreshed {
				if c.Auth.RefreshToken(ctx, a) {
					refreshed = true
					continue
				}
			}
			// Session IDs belong to this account. Refreshing its token is safe;
			// retrying the same ID under a different account is not.
		}
		attempts++
	}

	result.Success = false
	result.ErrorMessage = "delete session failed after retries"
	return result, errors.New(result.ErrorMessage)
}

// DeleteSessionForToken 直接使用 token 删除会话（直通模式）
func (c *Client) DeleteSessionForToken(ctx context.Context, token string, sessionID string) (*DeleteSessionResult, error) {
	clients := c.requestClientsFromContext(ctx)
	result := &DeleteSessionResult{
		SessionID: sessionID,
	}

	if sessionID == "" {
		result.ErrorMessage = "session_id is required"
		return result, errors.New(result.ErrorMessage)
	}

	headers := c.authHeadersFromContext(ctx, token)
	payload := map[string]any{
		"chat_session_id":  sessionID,
		"chat_session_ids": nil,
	}

	resp, status, err := c.postJSONWithStatus(ctx, clients.regular, clients.fallback, dsprotocol.DeepSeekDeleteSessionURL, headers, payload)
	if err != nil {
		result.ErrorMessage = err.Error()
		return result, err
	}

	if err := deleteResponseError(resp, status); err != nil {
		result.ErrorMessage = err.Error()
		return result, err
	}

	result.Success = true
	c.uploadCache.invalidateOwner(uploadReuseOwner(token))
	return result, nil
}

// DeleteAllSessions 删除所有会话（谨慎使用）
func (c *Client) DeleteAllSessions(ctx context.Context, a *auth.RequestAuth) error {
	clients := c.requestClientsForAuth(ctx, a)
	headers := c.authHeadersForAuth(a)
	resp, status, err := c.postJSONWithStatus(ctx, clients.regular, clients.fallback, dsprotocol.DeepSeekDeleteAllSessionsURL, headers, nil)
	if err != nil {
		config.Logger.Warn("[delete_all_sessions] request error", "error", err)
		return err
	}

	if err := deleteResponseError(resp, status); err != nil {
		return err
	}
	c.uploadCache.invalidateOwner(uploadReuseOwner(a.DeepSeekToken))
	return nil
}

// DeleteAllSessionsForToken 直接使用 token 删除所有会话（直通模式）
func (c *Client) DeleteAllSessionsForToken(ctx context.Context, token string) error {
	clients := c.requestClientsFromContext(ctx)
	headers := c.authHeadersFromContext(ctx, token)
	resp, status, err := c.postJSONWithStatus(ctx, clients.regular, clients.fallback, dsprotocol.DeepSeekDeleteAllSessionsURL, headers, nil)
	if err != nil {
		config.Logger.Warn("[delete_all_sessions_for_token] request error", "error", err)
		return err
	}

	if err := deleteResponseError(resp, status); err != nil {
		return err
	}
	c.uploadCache.invalidateOwner(uploadReuseOwner(token))
	return nil
}

func deleteResponseError(resp map[string]any, status int) error {
	data, hasData := resp["data"].(map[string]any)
	_, hasCode := resp["code"]
	_, hasBizCode := data["biz_code"]
	if !hasCode || !hasData || !hasBizCode {
		return errors.New("invalid deletion response: missing result codes")
	}
	code, bizCode, msg, bizMsg := extractResponseStatus(resp)
	if status != http.StatusOK || code != 0 || bizCode != 0 {
		return fmt.Errorf("deletion failed: status=%d, code=%d, biz_code=%d, msg=%s", status, code, bizCode, failureMessage(msg, bizMsg, "deletion failed"))
	}
	return nil
}
