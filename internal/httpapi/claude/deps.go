package claude

import (
	"context"
	"net/http"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
)

type AuthResolver interface {
	Determine(req *http.Request) (*auth.RequestAuth, error)
	Release(a *auth.RequestAuth)
}

// DeepSeekCaller 是 Claude handler 调用 DeepSeek 上游的最小接口集合。
// 比 OpenAI chat handler 的同名接口多需要 DeleteSessionForToken /
// DeleteAllSessionsForToken，用于支持 auto_delete.mode 与 -autodelete
// 后缀模型联动，对齐 App 抓包里的 chat_session/delete 行为。
type DeepSeekCaller interface {
	CreateSession(ctx context.Context, a *auth.RequestAuth, maxAttempts int) (string, error)
	GetPow(ctx context.Context, a *auth.RequestAuth, maxAttempts int) (string, error)
	UploadFile(ctx context.Context, a *auth.RequestAuth, req dsclient.UploadFileRequest, maxAttempts int) (*dsclient.UploadFileResult, error)
	CallCompletion(ctx context.Context, a *auth.RequestAuth, payload any, powResp string, maxAttempts int) (*http.Response, error)
	DeleteSessionForToken(ctx context.Context, token string, sessionID string) (*dsclient.DeleteSessionResult, error)
	DeleteAllSessionsForToken(ctx context.Context, token string) error
}

type ConfigReader interface {
	ModelAliases() map[string]string
	CurrentInputFileEnabled() bool
	CurrentInputFileMinChars() int
	AutoDeleteMode() string
}

type OpenAIChatRunner interface {
	ChatCompletions(w http.ResponseWriter, r *http.Request)
}

var _ AuthResolver = (*auth.Resolver)(nil)
var _ DeepSeekCaller = (*dsclient.Client)(nil)
var _ ConfigReader = (*config.Store)(nil)
