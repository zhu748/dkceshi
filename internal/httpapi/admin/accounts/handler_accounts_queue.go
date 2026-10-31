package accounts

import (
	"context"
	"ds2api/internal/devid"
	"net/http"
)

// queueStatus 处理 GET /admin/queue/status。
// 在号池状态之外附带设备 ID 模式与手工 device_id 剩余可用数量，
// 供账号管理界面实时展示"剩余可用device_id数量"标签。
func (h *Handler) queueStatus(w http.ResponseWriter, r *http.Request) {
	status := h.Pool.Status()
	status["device_profile_kind"] = "android"
	mode := h.Store.RuntimeDeviceIDMode()
	_, hasRefresher := h.realDeviceRefresher()
	status["real_device_id_refresh_supported"] = mode == "auto" && hasRefresher
	if checker, ok := h.DS.(interface {
		DeviceIDReadiness(context.Context) devid.Readiness
	}); ok && mode == "auto" {
		readiness := checker.DeviceIDReadiness(r.Context())
		status["device_id_sdk"] = readiness
		status["real_device_id_refresh_supported"] = hasRefresher && readiness.Ready
	}
	status["device_id_mode"] = mode
	status["device_id_remaining"] = h.Store.DeviceIDPoolSize()
	writeJSON(w, http.StatusOK, status)
}
