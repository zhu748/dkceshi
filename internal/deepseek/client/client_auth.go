package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"

	"ds2api/internal/auth"
	"ds2api/internal/banreport"
	"ds2api/internal/config"
	dsprotocol "ds2api/internal/deepseek/protocol"
)

// realRiskRotationLimit bounds how many times one Login call may regenerate
// an SDK device id after RISK_DEVICE_DETECTED. Each rotation costs a full
// collection round (10~25s plus the report), so the budget stays small; the
// store retains the last generated id either way, and repeatedly flagged
// fresh ids usually mean the egress or the account is the problem, not the id.
const realRiskRotationLimit = 2

func (c *Client) Login(ctx context.Context, acc config.Account) (string, error) {
	acc = c.loadAccountDeviceProfile(acc)
	rotations := 0
	for {
		var err error
		acc, err = c.ensureRiskDeviceID(ctx, acc)
		if err != nil {
			return "", err
		}
		token, err := c.loginOnce(ctx, acc)
		if err == nil {
			return token, nil
		}
		// SDK 签发的 id 被风控拒绝时自动换一台“新设备”重试：没有这条路径，
		// 一个被降权的 device_id 会永久不可用，只能人工重建档案。
		if !c.shouldRegenerateLoginDeviceID(acc, err) || rotations >= realRiskRotationLimit {
			return "", err
		}
		rotations++
		config.Logger.Warn("[device_id] regenerating real device id after RISK_DEVICE_DETECTED",
			"account", acc.Identifier(), "rotation", rotations)
		if _, rotErr := c.RefreshRealDeviceID(ctx, acc.Identifier(), true); rotErr != nil {
			// 档案没删掉就会算出同一个 id，重试没有意义：如实上报风控报错。
			config.Logger.Error("[device_id] real device id rotation failed",
				"account", acc.Identifier(), "error", rotErr)
			return "", err
		}
		acc = c.loadAccountDeviceProfile(acc)
	}
}

// loadAccountDeviceProfile 把账号在存储里的设备档案同步到本次登录用的账号副本。
func (c *Client) loadAccountDeviceProfile(acc config.Account) config.Account {
	if c.Store == nil {
		return acc
	}
	if profile, ok := c.Store.EnsureAccountDeviceProfile(acc.Identifier()); ok {
		acc.DeviceID = profile.DeviceID
		acc.DeviceIDType = profile.DeviceIDType
		acc.InstallationID = profile.InstallationID
		acc.AndroidID = profile.AndroidID
		acc.DeviceBrand = profile.DeviceBrand
		acc.Timezone = profile.Timezone
		acc.HIFDliq, acc.HIFDliqExpiresAt = profile.HIFDliq, profile.HIFDliqExpiresAt
		acc.HIFLeim, acc.HIFLeimExpiresAt = profile.HIFLeim, profile.HIFLeimExpiresAt
		acc.DeviceModel = profile.DeviceModel
		acc.AndroidAPILevel = profile.AndroidAPILevel
		if acc.RangersID == "" {
			acc.RangersID = profile.RangersID
		}
		if acc.Locale == "" {
			acc.Locale = profile.Locale
		}
	}
	return acc
}

// loginOnce 用账号当前携带的 device_id 执行一次登录。
func (c *Client) loginOnce(ctx context.Context, acc config.Account) (string, error) {
	clients := c.requestClientsForAccount(acc)
	payload := map[string]any{
		"password":  strings.TrimSpace(acc.Password),
		"device_id": loginDeviceID(acc),
		"os":        "android",
	}
	if email := strings.TrimSpace(acc.Email); email != "" {
		payload["email"] = email
	} else if mobile := strings.TrimSpace(acc.Mobile); mobile != "" {
		loginMobile, areaCode := normalizeMobileForLogin(mobile)
		payload["mobile"] = loginMobile
		payload["area_code"] = areaCode
	} else {
		return "", errors.New("missing email/mobile")
	}
	resp, err := c.postJSON(ctx, clients.regular, clients.fallback, dsprotocol.DeepSeekLoginURL, accountBaseHeaders(acc), payload)
	if err != nil {
		return "", err
	}
	code := intFrom(resp["code"])
	if code != 0 {
		return "", loginRejectedError("login failed: %v", resp["msg"])
	}
	data, _ := resp["data"].(map[string]any)
	if bizCode := intFrom(data["biz_code"]); bizCode != 0 {
		bizData, _ := data["biz_data"].(map[string]any)
		if sessionID, _ := bizData["mfa_session_id"].(string); bizCode == 14 && strings.TrimSpace(sessionID) != "" {
			// MFA is an unfinished verification flow, not rejected credentials.
			// Never expose or persist its session ID as an authenticated token.
			return "", &RequestFailure{Op: "login", Kind: FailureCaptchaRequired, Message: "multi-factor authentication required; complete verification in the official client and configure its token"}
		}
		bizMsg, _ := data["biz_msg"].(string)
		if isUserBannedResponse(bizCode, bizMsg) {
			c.persistBannedAccount(acc.Identifier(), "账户已被停用", banreport.SourceLogin)
		}
		return "", loginRejectedError("login failed: %v", data["biz_msg"])
	}
	bizData, _ := data["biz_data"].(map[string]any)
	user, _ := bizData["user"].(map[string]any)
	token, _ := user["token"].(string)
	if strings.TrimSpace(token) == "" {
		return "", loginRejectedError("missing login token")
	}
	if chat, _ := user["chat"].(map[string]any); chat != nil && intFrom(chat["is_muted"]) == 1 {
		until, _ := chat["mute_until"].(float64)
		c.persistMutedUntil(acc.Identifier(), until, banreport.SourceLogin)
	}
	if acc.Banned {
		c.clearBannedAccount(acc.Identifier())
	}
	c.noteLoginSuccess(acc.Identifier())
	// 对齐真实 App：登录后立即上报 check_device（App 启动流程 check_update 亦会
	// 重复调用）。上游可能借此轮换 token，返回值优先于登录响应里的原 token。
	if rotated, _ := c.CheckDevice(ctx, acc, token); rotated != "" {
		token = rotated
	}
	return token, nil
}

func (c *Client) CreateSession(ctx context.Context, a *auth.RequestAuth, maxAttempts int) (string, error) {
	if maxAttempts <= 0 {
		maxAttempts = c.maxRetries
	}
	attempts := 0
	for attempts < maxAttempts {
		clients := c.requestClientsForAuth(ctx, a)
		headers := c.authHeadersForAuth(a)
		// 真实 Android App 发送空 body（content-length: 0），项目对齐
		resp, status, err := c.postJSONWithStatus(ctx, clients.regular, clients.fallback, dsprotocol.DeepSeekCreateSessionURL, headers, nil)
		if err != nil {
			config.Logger.Warn("[create_session] request error", "error", err, "account", a.AccountID)
			attempts++
			if !retryAfterNetworkError(ctx, attempts, err) {
				return "", fmt.Errorf("create session failed: %w", err)
			}
			continue
		}
		code, bizCode, msg, bizMsg := extractResponseStatus(resp)
		if limited := fileRateLimit("create session", status, code, bizCode, msg, bizMsg); limited != nil {
			a.NoteRateLimit()
			return "", limited
		}
		if status == http.StatusOK && code == 0 && bizCode == 0 {
			sessionID := extractCreateSessionID(resp)
			if sessionID != "" {
				return sessionID, nil
			}
		}
		config.Logger.Warn("[create_session] failed", "status", status, "code", code, "biz_code", bizCode, "msg", msg, "biz_msg", bizMsg, "use_config_token", a.UseConfigToken, "account", a.AccountID)
		if kind := c.observeAccountFailure(a, resp, "request"); kind != FailureUnknown {
			if a.UseConfigToken && c.Auth != nil && c.Auth.SwitchAccount(ctx, a) {
				attempts++
				continue
			}
			return "", &RequestFailure{Op: "request", Kind: kind, Message: failureMessage(msg, bizMsg, "account unavailable")}
		}
		if c.recoverManagedAuth(ctx, a, &attempts, status, code, bizCode, msg, bizMsg) {
			continue
		}
		attempts++
	}
	return "", errors.New("create session failed")
}

func (c *Client) GetPow(ctx context.Context, a *auth.RequestAuth, maxAttempts int) (string, error) {
	return c.GetPowForTarget(ctx, a, dsprotocol.DeepSeekCompletionTargetPath, maxAttempts)
}

func (c *Client) GetPowForTarget(ctx context.Context, a *auth.RequestAuth, targetPath string, maxAttempts int) (string, error) {
	if maxAttempts <= 0 {
		maxAttempts = c.maxRetries
	}
	targetPath = strings.TrimSpace(targetPath)
	if targetPath == "" {
		targetPath = dsprotocol.DeepSeekCompletionTargetPath
	}
	// 1. 先查 PoW 预取缓存（复刻 App 行为：completion 后异步预取的 PoW）
	if a != nil {
		if pow, ok := c.powCache.Get(powCacheIdentity(a), targetPath); ok {
			config.Logger.Debug("[get_pow] cache hit", "account", a.AccountID, "target_path", targetPath)
			// 命中后异步触发下一次预取，复刻 App「取走即预取」行为
			c.powCache.SchedulePrefetch(ctx, a, targetPath, maxAttempts)
			return pow, nil
		}
	}
	// 2. 缓存 miss：同步获取
	pow, err := c.fetchPowSync(ctx, a, targetPath, maxAttempts)
	if err != nil {
		return "", err
	}
	// 3. 同步获取成功后也异步触发下一次预取（与 App 行为一致）
	if a != nil {
		c.powCache.SchedulePrefetch(ctx, a, targetPath, maxAttempts)
	}
	return pow, nil
}

// fetchPowSync 是真实的同步 PoW 获取实现（原本就是 GetPowForTarget 的主体逻辑）。
// 抽出来供 powPrefetchCache.fetcher 调用，避免循环依赖。
func (c *Client) fetchPowSync(ctx context.Context, a *auth.RequestAuth, targetPath string, maxAttempts int) (string, error) {
	solution, err := c.fetchPowSolution(ctx, a, targetPath, maxAttempts)
	return solution.header, err
}

func (c *Client) fetchPowSolution(ctx context.Context, a *auth.RequestAuth, targetPath string, maxAttempts int) (powSolution, error) {
	if maxAttempts <= 0 {
		maxAttempts = c.maxRetries
	}
	targetPath = strings.TrimSpace(targetPath)
	if targetPath == "" {
		targetPath = dsprotocol.DeepSeekCompletionTargetPath
	}
	attempts := 0
	lastFailureKind := FailureUnknown
	lastFailureMessage := ""
	for attempts < maxAttempts {
		clients := c.requestClientsForAuth(ctx, a)
		headers := c.authHeadersForAuth(a)
		resp, status, err := c.postJSONWithStatus(ctx, clients.regular, clients.fallback, dsprotocol.DeepSeekCreatePowURL, headers, map[string]any{"target_path": targetPath})
		if err != nil {
			config.Logger.Warn("[get_pow] request error", "error", err, "account", a.AccountID, "target_path", targetPath)
			lastFailureKind = FailureUnknown
			lastFailureMessage = err.Error()
			attempts++
			if !retryAfterNetworkError(ctx, attempts, err) {
				return powSolution{}, fmt.Errorf("get pow failed: %w", err)
			}
			continue
		}
		code, bizCode, msg, bizMsg := extractResponseStatus(resp)
		if limited := fileRateLimit("get pow", status, code, bizCode, msg, bizMsg); limited != nil {
			a.NoteRateLimit()
			return powSolution{}, limited
		}
		if status == http.StatusOK && code == 0 && bizCode == 0 {
			data, _ := resp["data"].(map[string]any)
			bizData, _ := data["biz_data"].(map[string]any)
			challenge, _ := bizData["challenge"].(map[string]any)
			receivedAt := time.Now()
			if challenge == nil {
				attempts++
				continue
			}
			// The response field is optional in APK ServerChatChallenge. The
			// solution always belongs to the path that requested the challenge.
			challenge["target_path"] = targetPath
			answer, err := ComputePow(ctx, challenge)
			if err != nil {
				attempts++
				continue
			}
			header, err := BuildPowHeader(challenge, answer)
			return powSolution{header: header, expiresAt: powChallengeExpiry(challenge, receivedAt)}, err
		}
		config.Logger.Warn("[get_pow] failed", "status", status, "code", code, "biz_code", bizCode, "msg", msg, "biz_msg", bizMsg, "use_config_token", a.UseConfigToken, "account", a.AccountID, "target_path", targetPath)
		lastFailureMessage = failureMessage(msg, bizMsg, "get pow failed")
		if isTokenInvalid(status, code, bizCode, msg, bizMsg) || isAuthIndicativeBizFailure(msg, bizMsg) {
			lastFailureKind = authFailureKind(a.UseConfigToken)
		} else {
			lastFailureKind = FailureUnknown
		}
		if kind := c.observeAccountFailure(a, resp, "request"); kind != FailureUnknown {
			if a.UseConfigToken && c.Auth != nil && c.Auth.SwitchAccount(ctx, a) {
				attempts++
				continue
			}
			return powSolution{}, &RequestFailure{Op: "request", Kind: kind, Message: failureMessage(msg, bizMsg, "account unavailable")}
		}
		if c.recoverManagedAuth(ctx, a, &attempts, status, code, bizCode, msg, bizMsg) {
			continue
		}
		attempts++
	}
	if lastFailureKind != FailureUnknown {
		return powSolution{}, &RequestFailure{Op: "get pow", Kind: lastFailureKind, Message: lastFailureMessage}
	}
	return powSolution{}, errors.New("get pow failed")
}

func (c *Client) authHeaders(token string) map[string]string {
	return c.authHeadersForAuth(&auth.RequestAuth{DeepSeekToken: token})
}

func (c *Client) authHeadersForAuth(a *auth.RequestAuth) map[string]string {
	if a == nil {
		return c.authHeaders("")
	}
	headers := BaseHeadersForAuth(a)
	headers["authorization"] = "Bearer " + a.DeepSeekToken
	return headers
}

func accountBaseHeaders(acc config.Account) map[string]string {
	return BaseHeadersForAuth(&auth.RequestAuth{Account: acc, AccountID: acc.Identifier()})
}

func accountHeaderSeed(a *auth.RequestAuth) string {
	if a == nil {
		return ""
	}
	if seed := accountIdentifierSeed(a.Account); seed != "" {
		return seed
	}
	return strings.TrimSpace(a.AccountID)
}

func accountIdentifierSeed(acc config.Account) string {
	if id := strings.TrimSpace(acc.Identifier()); id != "" {
		return strings.ToLower(id)
	}
	if email := strings.TrimSpace(acc.Email); email != "" {
		return strings.ToLower(email)
	}
	if mobile := strings.TrimSpace(acc.Mobile); mobile != "" {
		return strings.ToLower(mobile)
	}
	return ""
}

func loginDeviceID(acc config.Account) string {
	if v := strings.TrimSpace(acc.DeviceID); v != "" {
		return v
	}
	if v := strings.TrimSpace(dsprotocol.DefaultRiskDeviceID); v != "" {
		return v
	}
	return ""
}

func isTokenInvalid(status int, code int, bizCode int, msg string, bizMsg string) bool {
	msg = strings.ToLower(strings.TrimSpace(msg) + " " + strings.TrimSpace(bizMsg))
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return true
	}
	if code == 40001 || code == 40002 || code == 40003 || bizCode == 40001 || bizCode == 40002 || bizCode == 40003 {
		return true
	}
	return strings.Contains(msg, "token") ||
		strings.Contains(msg, "unauthorized") ||
		strings.Contains(msg, "expired") ||
		strings.Contains(msg, "not login") ||
		strings.Contains(msg, "login required") ||
		strings.Contains(msg, "invalid jwt")
}

func shouldAttemptRefresh(status int, code int, bizCode int, msg string, bizMsg string) bool {
	if isTokenInvalid(status, code, bizCode, msg, bizMsg) {
		return true
	}
	// Some DeepSeek failures come back as HTTP 200/code=0 but with non-zero biz_code.
	// Only attempt refresh when these biz failures still look auth-related.
	return status == http.StatusOK &&
		code == 0 &&
		bizCode != 0 &&
		isAuthIndicativeBizFailure(msg, bizMsg)
}

func isAuthIndicativeBizFailure(msg string, bizMsg string) bool {
	combined := strings.ToLower(strings.TrimSpace(msg) + " " + strings.TrimSpace(bizMsg))
	authKeywords := []string{
		"auth",
		"authorization",
		"credential",
		"expired",
		"invalid jwt",
		"jwt",
		"login",
		"not login",
		"session expired",
		"token",
		"unauthorized",
		"登录",
		"未登录",
		"认证",
		"凭证",
		"会话过期",
		"令牌",
	}
	for _, keyword := range authKeywords {
		if strings.Contains(combined, keyword) {
			return true
		}
	}
	return false
}

func authFailureKind(useConfigToken bool) FailureKind {
	if useConfigToken {
		return FailureManagedUnauthorized
	}
	return FailureDirectUnauthorized
}

func failureMessage(msg string, bizMsg string, fallback string) string {
	if trimmed := strings.TrimSpace(bizMsg); trimmed != "" {
		return trimmed
	}
	if trimmed := strings.TrimSpace(msg); trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(fallback)
}

// DeepSeek has returned create-session ids in both biz_data.id and
// biz_data.chat_session.id across observed response variants; accept either.
func extractCreateSessionID(resp map[string]any) string {
	data, _ := resp["data"].(map[string]any)
	bizData, _ := data["biz_data"].(map[string]any)
	if sessionID, _ := bizData["id"].(string); strings.TrimSpace(sessionID) != "" {
		return strings.TrimSpace(sessionID)
	}
	if chatSession, ok := bizData["chat_session"].(map[string]any); ok {
		if sessionID, _ := chatSession["id"].(string); strings.TrimSpace(sessionID) != "" {
			return strings.TrimSpace(sessionID)
		}
	}
	return ""
}

func normalizeMobileForLogin(raw string) (mobile string, areaCode any) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", nil
	}
	hasPlus := strings.HasPrefix(s, "+")
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	digits := b.String()
	if digits == "" {
		return "", nil
	}
	if (hasPlus || strings.HasPrefix(digits, "86")) && strings.HasPrefix(digits, "86") && len(digits) == 13 {
		return digits[2:], nil
	}
	return digits, nil
}
