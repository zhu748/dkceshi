package accounts

import (
	"ds2api/internal/account"
	"ds2api/internal/config"
	"ds2api/internal/devid"
	adminshared "ds2api/internal/httpapi/admin/shared"
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/url"
	"sort"
)

func (h *Handler) deleteAccount(w http.ResponseWriter, r *http.Request) {
	identifier := chi.URLParam(r, "identifier")
	if decoded, err := url.PathUnescape(identifier); err == nil {
		identifier = decoded
	}
	acc, ok := findAccountByIdentifier(h.Store, identifier)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"detail": "账号不存在"})
		return
	}
	identifier = acc.Identifier()
	unlock, err := devid.AcquireAccount(r.Context(), identifier)
	if err != nil {
		writeJSON(w, http.StatusRequestTimeout, map[string]any{"detail": err.Error()})
		return
	}
	defer unlock()
	var removedEmail, removedMobile string
	busy := h.inflightAccounts()
	err = h.Store.Update(func(c *config.Config) error {
		idx := -1
		for i, a := range c.Accounts {
			if accountMatchesIdentifier(a, identifier) {
				idx = i
				break
			}
		}
		if idx < 0 {
			return fmt.Errorf("账号不存在")
		}
		removedEmail = c.Accounts[idx].Email
		removedMobile = c.Accounts[idx].Mobile
		c.Accounts = append(c.Accounts[:idx], c.Accounts[idx+1:]...)
		if c.ElasticPool.Enabled {
			account.ReconcileElasticPoolWithBusy(c, busy)
		}
		// 账号被删除后重算号池绑定数。
		config.ReconcileDeviceIDBindings(c)
		return nil
	})
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"detail": err.Error()})
		return
	}
	removeDeviceProfile(config.Account{Email: removedEmail, Mobile: removedMobile}.Identifier(), h.Store.Accounts())
	forgetAuthFailures(h.DS, config.Account{Email: removedEmail, Mobile: removedMobile}.Identifier())
	h.Pool.Reset()
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "total_accounts": len(h.Store.Snapshot().Accounts)})
}

// removeDeviceProfile 删除账号的设备档案文件；文件不存在时静默返回。
func removeDeviceProfile(identifier string, accounts []config.Account) {
	if identifier == "" {
		return
	}
	if err := devid.RemoveProfiles(identifier, accounts); err != nil {
		config.Logger.Warn("[device_id] remove profile failed", "account", identifier, "error", err)
	}
}

// authFailureForgetter 由 deepseek client 可选实现，用于清理账号的连续鉴权失败计数。
type authFailureForgetter interface {
	ForgetAuthFailures(identifier string)
}

// forgetAuthFailures 删除账号后清理其连续鉴权失败计数（DS 未实现时静默跳过）。
func forgetAuthFailures(ds adminshared.DeepSeekCaller, identifier string) {
	if ds == nil || identifier == "" {
		return
	}
	if fg, ok := ds.(authFailureForgetter); ok {
		fg.ForgetAuthFailures(identifier)
	}
}

// batchDeleteBannedAccounts 处理 POST /admin/accounts/banned/delete 请求。
// 删除所有被上游停用(IsBanned)的账号，并清理其设备档案与 device_id 生成锁。
func (h *Handler) batchDeleteBannedAccounts(w http.ResponseWriter, r *http.Request) {

	candidates := make([]string, 0)
	for _, acc := range h.Store.Accounts() {
		if acc.IsBanned() {
			candidates = append(candidates, acc.Identifier())
		}
	}
	sort.Strings(candidates)
	locked := make(map[string]bool, len(candidates))
	var unlockers []func()
	defer func() {
		for i := len(unlockers) - 1; i >= 0; i-- {
			unlockers[i]()
		}
	}()
	for _, id := range candidates {
		unlock, err := devid.AcquireAccount(r.Context(), id)
		if err != nil {
			writeJSON(w, http.StatusRequestTimeout, map[string]any{"detail": err.Error()})
			return
		}
		unlockers = append(unlockers, unlock)
		locked[id] = true
	}
	var removed []string
	busy := h.inflightAccounts()
	err := h.Store.Update(func(c *config.Config) error {
		kept := make([]config.Account, 0, len(c.Accounts))
		for _, acc := range c.Accounts {
			if acc.IsBanned() && locked[acc.Identifier()] {
				if id := acc.Identifier(); id != "" {
					removed = append(removed, id)
				}
				continue
			}
			kept = append(kept, acc)
		}
		c.Accounts = kept
		if c.ElasticPool.Enabled {
			account.ReconcileElasticPoolWithBusy(c, busy)
		}
		// 被停用账号被删除后重算号池绑定数。
		config.ReconcileDeviceIDBindings(c)
		return nil
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": err.Error()})
		return
	}
	for _, identifier := range removed {
		removeDeviceProfile(identifier, h.Store.Accounts())
		forgetAuthFailures(h.DS, identifier)
	}
	h.Pool.Reset()
	h.notifyAccountsChanged()
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "total": len(removed)})
}
