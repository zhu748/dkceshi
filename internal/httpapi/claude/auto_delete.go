package claude

import (
	"context"
	"time"

	"ds2api/internal/auth"
	"ds2api/internal/config"
)

// autoDeleteRemoteSession 在 Claude /v1/messages 请求结束后按全局 auto_delete.mode
// 与模型后缀策略清理远端会话。
//
// 策略组合（与 OpenAI chat handler 完全对齐）：
//   - 全局 mode=single：删除本次会话（无条件）
//   - 全局 mode=all：清空账号全部会话（无条件）
//   - 全局 mode=none + 模型带 -autodelete 后缀：降级为删除本次会话
//   - 全局 mode=none + 普通模型：不删除
//
// 设计意图：让 Claude 入口也能通过 -autodelete 后缀模型（如
// claude-sonnet-4-6-autodelete）在全局关闭删除策略的前提下，
// 对个别请求按需启用「仅删除本次对话」，对应 App 抓包里的
// POST /api/v0/chat_session/delete 动作。
func (h *Handler) autoDeleteRemoteSession(ctx context.Context, a *auth.RequestAuth, sessionID string, resolvedModel string) {
	mode := h.Store.AutoDeleteMode()
	suffixAutoDelete := config.IsAutoDeleteModel(resolvedModel)

	// 计算最终生效模式
	effectiveMode := mode
	if mode == "none" && suffixAutoDelete {
		effectiveMode = "single"
	}
	if effectiveMode == "none" || a == nil || a.DeepSeekToken == "" {
		return
	}

	deleteBaseCtx := context.WithoutCancel(ctx)
	deleteCtx, cancel := context.WithTimeout(deleteBaseCtx, 10*time.Second)
	defer cancel()

	switch effectiveMode {
	case "single":
		if sessionID == "" {
			config.Logger.Warn("[claude_auto_delete] skipped single-session delete because session_id is empty", "account", a.AccountID, "model", resolvedModel)
			return
		}
		_, err := h.DS.DeleteSessionForToken(deleteCtx, a.DeepSeekToken, sessionID)
		if err != nil {
			config.Logger.Warn("[claude_auto_delete] failed", "account", a.AccountID, "mode", mode, "effective_mode", effectiveMode, "model", resolvedModel, "session_id", sessionID, "error", err)
			return
		}
		config.Logger.Debug("[claude_auto_delete] success", "account", a.AccountID, "mode", mode, "effective_mode", effectiveMode, "model", resolvedModel, "session_id", sessionID)
	case "all":
		if err := h.DS.DeleteAllSessionsForToken(deleteCtx, a.DeepSeekToken); err != nil {
			config.Logger.Warn("[claude_auto_delete] failed", "account", a.AccountID, "mode", mode, "model", resolvedModel, "error", err)
			return
		}
		config.Logger.Debug("[claude_auto_delete] success", "account", a.AccountID, "mode", mode, "model", resolvedModel)
	default:
		config.Logger.Warn("[claude_auto_delete] unknown mode", "account", a.AccountID, "mode", mode, "model", resolvedModel)
	}
}
