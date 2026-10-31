package chat

import (
	"ds2api/internal/assistantturn"
	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
	"net/http"
	"strings"
)

// recordVercelStreamCompletion 在 Vercel Node 流式路径收尾（release）时
// 把最终结果回填到聊天历史并记录 Token 用量。Node 侧会在 release 请求里
// 携带 finish_reason/usage/thinking/content/error。
func (h *Handler) recordVercelStreamCompletion(r *http.Request, a *auth.RequestAuth, stdReq promptcompat.StandardRequest, req map[string]any) {
	if a == nil {
		return
	}
	finishReason := strings.TrimSpace(asString(req["finish_reason"]))
	if finishReason == "" {
		finishReason = "stop"
	}
	thinking := asString(req["thinking"])
	content := asString(req["content"])
	errorMessage := strings.TrimSpace(asString(req["error"]))
	usage, _ := req["usage"].(map[string]any)

	// 流尚未建立就失败（例如上游连接失败）的 release 不产生任何有意义的
	// 记录，跳过以免写入空的历史条目。
	if usage == nil && thinking == "" && content == "" && errorMessage == "" {
		return
	}

	session := startChatHistoryWithUsage(h.ChatHistory, h.Usage, r, a, stdReq)
	if session == nil {
		return
	}
	if errorMessage != "" {
		if asString(req["error_code"]) == assistantturn.OutputCodePromptEcho {
			session.error(http.StatusBadGateway, errorMessage, assistantturn.OutputCodePromptEcho, thinking, content)
			return
		}
		session.error(http.StatusInternalServerError, errorMessage, finishReason, thinking, content)
		return
	}
	if finishReason == "stopped" {
		session.stopped(thinking, content, finishReason)
		return
	}
	session.success(http.StatusOK, thinking, content, finishReason, usage)
}
