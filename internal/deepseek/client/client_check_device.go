package client

import (
	"context"
	"strings"

	"ds2api/internal/config"
	dsprotocol "ds2api/internal/deepseek/protocol"
)

// loginDeviceModel 返回 check_device 请求体使用的设备型号，取值优先级与
// loginDeviceID 一致：账号字段 → 全局基础头 → 空。
func loginDeviceModel(acc config.Account) string {
	if v := strings.TrimSpace(acc.DeviceModel); v != "" {
		return v
	}
	if v := strings.TrimSpace(accountBaseHeaders(acc)["x-device-model"]); v != "" {
		return v
	}
	return ""
}

// CheckDevice 对齐真实 Android App 2.7.1 的启动行为：登录成功后携带当前
// token 调用 POST /api/v0/users/auth_token/check_device，请求体为
// {"device_model": ..., "device_id": ...}（字段序与 App 的
// CheckDeviceRequest 一致）。服务端在响应 biz_data.rotate 中可能下发轮换
// 后的新 token，此时返回新 token，调用方应改用并持久化它；未轮换时返回
// 空串。
//
// 失败策略与 App 对齐：check_device 是加固动作而非登录的必要步骤，网络或
// 业务失败只记录告警并返回空串，绝不让登录流程因它失败。
func (c *Client) CheckDevice(ctx context.Context, acc config.Account, token string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", nil
	}
	deviceID := accountBaseHeaders(acc)["x-device-id"]
	if deviceID == "" {
		return "", nil
	}
	deviceModel := loginDeviceModel(acc)
	if deviceModel == "" {
		deviceModel = "android"
	}

	clients := c.requestClientsForAccount(acc)
	headers := accountBaseHeaders(acc)
	headers["authorization"] = "Bearer " + token
	payload := map[string]any{
		"device_model": deviceModel,
		"device_id":    deviceID,
	}

	resp, status, err := c.postJSONWithStatus(ctx, clients.regular, clients.fallback, dsprotocol.DeepSeekCheckDeviceURL, headers, payload)
	if err != nil {
		config.Logger.Warn("[check_device] request error", "error", err, "account", acc.Identifier())
		return "", nil
	}
	code := intFrom(resp["code"])
	if status != 200 || code != 0 {
		msg, _ := resp["msg"].(string)
		config.Logger.Warn("[check_device] rejected", "status", status, "code", code, "msg", msg, "account", acc.Identifier())
		return "", nil
	}
	data, _ := resp["data"].(map[string]any)
	if data == nil {
		return "", nil
	}
	if bizCode := intFrom(data["biz_code"]); bizCode != 0 {
		bizMsg, _ := data["biz_msg"].(string)
		config.Logger.Warn("[check_device] biz rejected", "biz_code", bizCode, "biz_msg", bizMsg, "account", acc.Identifier())
		return "", nil
	}
	bizData, _ := data["biz_data"].(map[string]any)
	rotate, _ := bizData["rotate"].(map[string]any)
	if rotate == nil {
		return "", nil
	}
	newToken, _ := rotate["token"].(string)
	newToken = strings.TrimSpace(newToken)
	if newToken == "" {
		return "", nil
	}
	config.Logger.Info("[check_device] token rotated by upstream", "account", acc.Identifier())
	return newToken, nil
}
