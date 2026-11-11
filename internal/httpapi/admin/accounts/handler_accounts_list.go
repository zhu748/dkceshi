package accounts

import (
	"net/http"
	"strings"

	"ds2api/internal/config"
	adminshared "ds2api/internal/httpapi/admin/shared"
)

func (h *Handler) listAccounts(w http.ResponseWriter, r *http.Request) {
	page := intFromQuery(r, "page", 1)
	pageSize := intFromQuery(r, "page_size", 10)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 1
	}
	if pageSize > 5000 {
		pageSize = 5000
	}
	accounts := h.Store.Accounts()
	counts := accountStatusCounts(accounts)
	statusFilter := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("status")))
	q := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("q")))
	indices := orderedAccountIndices(accounts, true, func(acc config.Account) bool {
		return matchesAccountStatusFilter(acc, statusFilter) && matchesAccountQuery(acc, q)
	}, config.Account.IsSchedulable)
	total := len(indices)
	totalPages := 1
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	start, end := adminshared.PageBounds(page, pageSize, total)
	items := make([]map[string]any, 0, end-start)
	for _, index := range indices[start:end] {
		acc := accounts[index]
		testStatus, _ := h.Store.AccountTestStatus(acc.Identifier())
		token := strings.TrimSpace(acc.Token)
		items = append(items, map[string]any{
			"identifier":     acc.Identifier(),
			"name":           acc.Name,
			"remark":         acc.Remark,
			"email":          acc.Email,
			"mobile":         acc.Mobile,
			"proxy_id":       acc.ProxyID,
			"manual_binding": acc.ManualBinding,
			"no_proxy":       acc.NoProxy,
			"pool_type":      config.NormalizePoolType(acc.PoolType),
			"priority":       acc.Priority,
			"has_password":   acc.Password != "",
			"has_token":      token != "",
			"token_preview":  maskSecretPreview(token),
			"test_status":    testStatus,
			"device_id_type": config.NormalizeDeviceIDType(acc.DeviceIDType),
			"device_model":   acc.DeviceModel, "android_api_level": acc.AndroidAPILevel,
			"rangers_id": acc.RangersID, "device_id": acc.DeviceID,
			"installation_id": acc.InstallationID, "android_id": acc.AndroidID,
			"device_brand": acc.DeviceBrand, "timezone": acc.Timezone,
			"photo_compress_ratio": acc.PhotoCompressRatio, "picture_compress_format": acc.PictureCompressFormat,
			"cooldown_until": acc.CooldownUntil, "manual_disabled": acc.ManualDisabled,
			"cooling_down": acc.IsCoolingDown(), "schedulable": acc.IsSchedulable(),
			"enabled":         acc.IsEnabled(),
			"disabled_reason": acc.DisabledReason,
			"banned":          acc.IsBanned(),
			"auth_failed":     acc.IsAuthFailed(),
			"muted":           acc.IsMuted(),
			"muted_until":     acc.MutedUntil,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":       items,
		"total":       total,
		"page":        page,
		"page_size":   pageSize,
		"total_pages": totalPages,
		"counts":      counts,
	})
}
