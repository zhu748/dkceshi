package responsehistory

import (
	"net/http"
	"time"

	"ds2api/internal/chathistory"
	"ds2api/internal/promptcompat"
	"ds2api/internal/usagestats"
)

// Stopped 关闭一个被客户端取消的流式会话：把条目落为 "stopped" 终态，并按已
// 生成的部分内容估算 token 用量入账，避免条目永久停留在 "streaming" 且账本
// 漏记已消耗的请求（对齐 openai/chat 表面的 stopped 语义）。
func (s *Session) Stopped(thinking, content, finishReason string, current ...promptcompat.StandardRequest) {
	if s == nil {
		return
	}
	request := s.usageRequest
	if request.PromptTokenText == "" && request.FinalPrompt == "" {
		request.FinalPrompt = s.startParams.FinalPrompt
	}
	if request.ResolvedModel == "" {
		request.ResolvedModel = s.recordModel
	}
	usage := usagestats.PartialUsage(request, thinking, content, current...)
	s.recordUsage(usage)
	if s.store == nil || s.disabled {
		return
	}
	s.persistUpdate(chathistory.UpdateParams{
		Status:           "stopped",
		ReasoningContent: thinking,
		Content:          content,
		StatusCode:       http.StatusOK,
		ElapsedMs:        time.Since(s.startedAt).Milliseconds(),
		FinishReason:     finishReason,
		Usage:            usage,
		Completed:        true,
	})
}
