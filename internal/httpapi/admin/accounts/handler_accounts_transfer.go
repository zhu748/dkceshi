package accounts

import (
	"errors"
	"net/http"
	"time"

	"ds2api/internal/config"
)

func (h *Handler) exportAccounts(w http.ResponseWriter, _ *http.Request) {
	accounts := h.Store.Accounts()
	if accounts == nil {
		accounts = []config.Account{}
	}
	now := time.Now().UTC()
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", `attachment; filename="ds2api-accounts-`+now.Format("20060102-150405")+`.json"`)
	writeJSON(w, http.StatusOK, accountBackup{Format: accountBackupFormat, Version: 1, ExportedAt: now.Format(time.RFC3339), Accounts: accounts})
}

func (h *Handler) importAccounts(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAccountImportBytes)
	incoming, err := decodeAccountBackup(r.Body)
	if err != nil {
		status := http.StatusBadRequest
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		writeJSON(w, status, map[string]any{"detail": err.Error()})
		return
	}
	added, updated, total := 0, 0, 0
	err = h.Store.Update(func(c *config.Config) error {
		accounts, a, u, mergeErr := mergeImportedAccounts(c.Accounts, incoming)
		if mergeErr != nil {
			return newRequestError(mergeErr.Error())
		}
		if err := config.ValidateAccountProxyReferences(accounts, c.Proxies); err != nil {
			return newRequestError("账号引用的代理不存在，请先导入对应代理配置: " + err.Error())
		}
		c.Accounts = accounts
		added, updated, total = a, u, len(accounts)
		return nil
	})
	if err != nil {
		status := http.StatusInternalServerError
		detail := err.Error()
		if message, ok := requestErrorDetail(err); ok {
			status, detail = http.StatusBadRequest, message
		}
		writeJSON(w, status, map[string]any{"detail": detail})
		return
	}
	if h.Pool != nil {
		h.Pool.Reset()
		h.Pool.SyncRateLimits()
		h.Pool.SyncDailyLimits()
	}
	h.notifyAccountsChanged()
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "added": added, "updated": updated, "total_accounts": total})
}
