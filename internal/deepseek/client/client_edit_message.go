package client

import (
	"context"
	"fmt"
	"net/http"
	"time"

	dsprotocol "ds2api/internal/deepseek/protocol"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
)

// EditMessageResult 包含 edit_message 调用的结果信息。
type EditMessageResult struct {
	// Response 是 SSE 流式响应体。
	Response *http.Response
	// RequestMessageID 是本次 edit 产生的新用户消息 ID，
	// 用于下次 edit_message 的 message_id 参数。
	RequestMessageID int
	// ResponseMessageID 是模型回复的消息 ID。
	ResponseMessageID int
}

// CallEditMessage 调用 DeepSeek 的 /api/v0/chat/edit_message 端点，
// 在已有会话中编辑（重发）用户消息，复用远端会话上下文。
//
// 与 CallCompletion 不同，edit_message 只发送新的用户消息文本，
// 不需要重发完整对话历史——远端已有完整上下文。
// 请求也需要 PoW header 和认证 header。
func (c *Client) CallEditMessage(ctx context.Context, a *auth.RequestAuth, payload any, powResp string, maxAttempts int) (*http.Response, error) {
	_ = maxAttempts
	clients := c.requestClientsForAuth(ctx, a)
	headers := c.authHeadersForAuth(a)
	headers["x-ds-pow-response"] = powResp

	resp, err := c.streamPostOnce(ctx, clients.stream, dsprotocol.DeepSeekEditMessageURL, headers, payload)
	if err != nil {
		return nil, err
	}

	// edit_message 成功后也异步预取 PoW
	if resp.StatusCode == http.StatusOK {
		if a != nil {
			c.powCache.SchedulePrefetch(context.Background(), a, dsprotocol.DeepSeekCompletionTargetPath, c.maxRetries)
		}
	}
	return resp, nil
}

// generateClientStreamID 生成格式为 YYYYMMDD-<hex> 的客户端流 ID，
// 对齐 Android App 的 client_stream_id 格式。
func generateClientStreamID() string {
	now := time.Now()
	datePart := now.Format("20060102")
	// 使用 unix 纳秒的低 48 位作为 hex 部分
	nano := now.UnixNano()
	hexPart := fmt.Sprintf("%012x", nano&0xFFFFFFFFFFFF)
	return datePart + "-" + hexPart
}

// 静态断言：确认 Client 满足 shared.DeepSeekCaller 接口
var _ interface {
	CreateSession(ctx context.Context, a *auth.RequestAuth, maxAttempts int) (string, error)
	GetPow(ctx context.Context, a *auth.RequestAuth, maxAttempts int) (string, error)
	UploadFile(ctx context.Context, a *auth.RequestAuth, req UploadFileRequest, maxAttempts int) (*UploadFileResult, error)
	CallCompletion(ctx context.Context, a *auth.RequestAuth, payload any, powResp string, maxAttempts int) (*http.Response, error)
	CallEditMessage(ctx context.Context, a *auth.RequestAuth, payload any, powResp string, maxAttempts int) (*http.Response, error)
	DeleteSessionForToken(ctx context.Context, token string, sessionID string) (*DeleteSessionResult, error)
	DeleteAllSessionsForToken(ctx context.Context, token string) error
} = (*Client)(nil)

// 确保引用的包被使用
var (
	_ *auth.RequestAuth
	_ = promptcompat.NewOrderedJSONMap
)
