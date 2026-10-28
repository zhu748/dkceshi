package client

import (
	"ds2api/internal/account"
	"ds2api/internal/auth"
	"ds2api/internal/banreport"
	"ds2api/internal/config"
	"os"
	"strconv"
	"strings"
	"time"
)

func (c *Client) persistMutedUntil(identifier string, muteUntil float64, source string) {
	if c == nil || c.Store == nil || strings.TrimSpace(identifier) == "" {
		return
	}
	if muteUntil <= 0 {
		// 上游未下发到期时间戳时的兜底禁言时长（10分钟）
		muteUntil = float64(time.Now().Add(10 * time.Minute).Unix())
	}
	busy := c.inflightAccounts()
	if err := c.Store.Update(func(cfg *config.Config) error {
		for i := range cfg.Accounts {
			if cfg.Accounts[i].Identifier() != identifier {
				continue
			}
			cfg.Accounts[i].MutedUntil = muteUntil
			break
		}
		if cfg.ElasticPool.Enabled {
			account.ReconcileElasticPoolWithBusy(cfg, busy)
		}
		// 被禁言的账号不再持有 device_id 绑定，下次重新启用后重新分配。
		/* fixed Android profiles are retained */
		return nil
	}); err != nil {
		config.Logger.Error("[muted_account] failed to persist muted_until", "account", identifier, "error", err)
		return
	}
	if c.Auth != nil && c.Auth.Pool != nil {
		c.Auth.Pool.Reset()
	}
	// 弹性号池可能借机启用补位账号，通知桥立即按已有测速结果分配节点。
	c.notifyAccountPoolChanged()
	// 在持久化之后上报：此时 enabled_count 已经扣除了本次被禁言的账号。
	banreport.Report(c.Store, banreport.Event{
		Type:      banreport.KindMuted,
		Account:   identifier,
		MuteUntil: muteUntil,
		Source:    source,
	})
}

// isUserBannedResponse 判断登录响应是否表示账号被上游停用（USER_IS_BANNED）。
func isUserBannedResponse(bizCode int, bizMsg string) bool {
	msg := strings.ToLower(strings.TrimSpace(bizMsg))
	return strings.Contains(msg, "user_is_banned") || bizCode == 10
}

// persistBannedAccount 持久化账号因上游 USER_IS_BANNED 而被停用的状态。
// 与禁言不同：禁言有 MutedUntil 可自动恢复，而被上游停用需等待
// 下次成功刷新 token 后才会解除。
// source 标记检测点，用于封号上报（见 internal/banreport）。
func (c *Client) persistBannedAccount(identifier string, reason string, source string) {
	if c == nil || c.Store == nil || strings.TrimSpace(identifier) == "" || strings.TrimSpace(reason) == "" {
		return
	}
	busy := c.inflightAccounts()
	if err := c.Store.Update(func(cfg *config.Config) error {
		for i := range cfg.Accounts {
			if cfg.Accounts[i].Identifier() != identifier {
				continue
			}
			cfg.Accounts[i].Banned = true
			cfg.Accounts[i].Disabled = true
			cfg.Accounts[i].DisabledReason = reason
			break
		}
		if cfg.ElasticPool.Enabled {
			account.ReconcileElasticPoolWithBusy(cfg, busy)
		}
		// 被封禁的账号不再持有 device_id 绑定。
		/* fixed Android profiles are retained */
		return nil
	}); err != nil {
		config.Logger.Error("[banned_account] failed to persist banned state", "account", identifier, "error", err)
		return
	}
	if c.Auth != nil && c.Auth.Pool != nil {
		c.Auth.Pool.Reset()
	}
	c.notifyAccountPoolChanged()
	config.Logger.Warn("[banned_account] account disabled after USER_IS_BANNED", "account", identifier, "reason", reason)
	// 在持久化之后上报：此时 enabled_count 已经扣除了本次被封号的账号。
	banreport.Report(c.Store, banreport.Event{
		Type:    banreport.KindBanned,
		Account: identifier,
		Source:  source,
	})
}

// clearBannedAccount 在成功刷新 token 后解除上游停用状态。
func (c *Client) clearBannedAccount(identifier string) {
	if c == nil || c.Store == nil || strings.TrimSpace(identifier) == "" {
		return
	}
	busy := c.inflightAccounts()
	if err := c.Store.Update(func(cfg *config.Config) error {
		for i := range cfg.Accounts {
			if cfg.Accounts[i].Identifier() != identifier {
				continue
			}
			if !cfg.Accounts[i].Banned {
				return nil
			}
			cfg.Accounts[i].Banned = false
			cfg.Accounts[i].Disabled = cfg.Accounts[i].ManualDisabled
			cfg.Accounts[i].DisabledReason = ""
			break
		}
		if cfg.ElasticPool.Enabled {
			account.ReconcileElasticPoolWithBusy(cfg, busy)
		}
		return nil
	}); err != nil {
		config.Logger.Error("[banned_account] failed to clear banned state", "account", identifier, "error", err)
		return
	}
	if c.Auth != nil && c.Auth.Pool != nil {
		c.Auth.Pool.Reset()
	}
	c.notifyAccountPoolChanged()
	config.Logger.Info("[banned_account] account re-enabled after successful token refresh", "account", identifier)
}

// captchaCooldown is how long an account is benched after a captcha challenge.
// A challenge means risk control has already flagged this account; continuing
// to drive traffic through it is the fastest way to escalate to a mute.
// Override with DS2API_CAPTCHA_COOLDOWN_MINUTES.
const defaultCaptchaCooldownMinutes = 30

func captchaCooldownDuration() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("DS2API_CAPTCHA_COOLDOWN_MINUTES")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			return time.Duration(n) * time.Minute
		}
	}
	return defaultCaptchaCooldownMinutes * time.Minute
}

// coolDownAfterCaptcha benches the account and reports whether it was applied.
func (c *Client) coolDownAfterCaptcha(a *auth.RequestAuth, op string) {
	if c == nil || a == nil || !a.UseConfigToken {
		return
	}
	identifier := strings.TrimSpace(a.AccountID)
	if identifier == "" {
		return
	}
	cooldown := captchaCooldownDuration()
	if cooldown <= 0 {
		return
	}
	until := float64(time.Now().Add(cooldown).Unix())
	c.persistCooldownUntil(identifier, until)
	a.Account.CooldownUntil = until
	config.Logger.Warn("[captcha] account cooled down after challenge",
		"op", op, "account", identifier, "cooldown_minutes", int(cooldown.Minutes()), "until", until)
}

// persistCooldownUntil stores the cooldown deadline, mirroring how a mute is
// persisted so an elastic pool can promote a standby account in its place.
func (c *Client) persistCooldownUntil(identifier string, until float64) {
	if c == nil || c.Store == nil || strings.TrimSpace(identifier) == "" || until <= 0 {
		return
	}
	busy := c.inflightAccounts()
	if err := c.Store.Update(func(cfg *config.Config) error {
		for i := range cfg.Accounts {
			if cfg.Accounts[i].Identifier() != identifier {
				continue
			}
			cfg.Accounts[i].CooldownUntil = until
			break
		}
		if cfg.ElasticPool.Enabled {
			account.ReconcileElasticPoolWithBusy(cfg, busy)
		}
		return nil
	}); err != nil {
		config.Logger.Error("[captcha] failed to persist cooldown_until", "account", identifier, "error", err)
		return
	}
	if c.Auth != nil && c.Auth.Pool != nil {
		c.Auth.Pool.Reset()
	}
	// 弹性号池可能借机启用补位账号，通知桥立即按已有测速结果分配节点。
	c.notifyAccountPoolChanged()
}

func (c *Client) inflightAccounts() map[string]bool {
	if c != nil && c.Auth != nil && c.Auth.Pool != nil {
		return c.Auth.Pool.InUseIDs()
	}
	return nil
}

func (c *Client) SetAccountPoolChanged(fn func()) {
	if c == nil {
		return
	}
	c.poolChangedMu.Lock()
	c.poolChanged = fn
	c.poolChangedMu.Unlock()
}

func (c *Client) notifyAccountPoolChanged() {
	if c == nil {
		return
	}
	c.poolChangedMu.RLock()
	fn := c.poolChanged
	c.poolChangedMu.RUnlock()
	if fn != nil {
		fn()
	}
}
