package client

import "strings"

func extractResponseStatus(resp map[string]any) (code int, bizCode int, msg string, bizMsg string) {
	code = intFrom(resp["code"])
	msg, _ = resp["msg"].(string)
	data, _ := resp["data"].(map[string]any)
	bizCode = intFrom(data["biz_code"])
	bizMsg, _ = data["biz_msg"].(string)
	if strings.TrimSpace(bizMsg) == "" {
		if bizData, ok := data["biz_data"].(map[string]any); ok {
			bizMsg, _ = bizData["msg"].(string)
		}
	}
	return code, bizCode, msg, bizMsg
}
