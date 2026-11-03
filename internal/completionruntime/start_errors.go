package completionruntime

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"ds2api/internal/assistantturn"
	"ds2api/internal/auth"
	dsclient "ds2api/internal/deepseek/client"
)

func accountFailureOutputError(err error) *assistantturn.OutputError {
	var limited *dsclient.FileRateLimitError
	if errors.As(err, &limited) || dsclient.IsBannedError(err) || dsclient.IsMutedError(err) {
		return completionCallOutputError(context.Background(), err)
	}
	return nil
}

// sessionOutputError 把 CreateSession 失败映射为标准输出错误。只有上游明确以
// 鉴权失败拒绝（托管账号的 FailureManagedUnauthorized、直传 token 的
// FailureDirectUnauthorized）才标记 account_unauthorized / 直传 token 错误，
// 让共享重试逻辑刷新 Token 或换号；网络抖动等非鉴权失败归类为可换号重试的
// upstream_unavailable，避免把健康账号误判为鉴权异常。
func sessionOutputError(a *auth.RequestAuth, err error) *assistantturn.OutputError {
	if typed := accountFailureOutputError(err); typed != nil {
		return typed
	}
	if dsclient.IsManagedUnauthorizedError(err) || dsclient.IsDirectUnauthorizedError(err) {
		return authOutputError(a)
	}
	if a != nil && !a.UseConfigToken {
		return authOutputError(a)
	}
	return &assistantturn.OutputError{Status: http.StatusBadGateway, Message: "Failed to create session.", Code: "upstream_unavailable"}
}

const DirectTokenErrorMessage = "Invalid token. If this should be a DS2API key, add it to config.keys first."

// powOutputError 把 GetPow 失败映射为输出错误。只有上游明确以鉴权失败拒绝托管
// 账号时才带 account_unauthorized 错误码，让共享重试逻辑先刷新 Token 再换号；
// 网络抖动等非鉴权失败归类为可换号重试的 upstream_unavailable，避免把健康账号
// 误判为鉴权异常（isManagedUnauthorized 只认 account_unauthorized 错误码）。
// 直传 token 模式保持原有 401 语义。
func powOutputError(a *auth.RequestAuth, err error) *assistantturn.OutputError {
	if typed := accountFailureOutputError(err); typed != nil {
		return typed
	}
	const message = "Failed to get PoW (invalid token or unknown error)."
	if a == nil || !a.UseConfigToken {
		return &assistantturn.OutputError{Status: http.StatusUnauthorized, Message: message, Code: "error"}
	}
	if dsclient.IsManagedUnauthorizedError(err) {
		return &assistantturn.OutputError{Status: http.StatusUnauthorized, Message: message, Code: OutputCodeAccountUnauthorized}
	}
	return &assistantturn.OutputError{Status: http.StatusBadGateway, Message: message, Code: "upstream_unavailable"}
}

func IsDirectTokenAuthError(outErr *assistantturn.OutputError) bool {
	return outErr != nil && outErr.Status == http.StatusUnauthorized && outErr.Message == DirectTokenErrorMessage
}

func Errorf(status int, format string, args ...any) *assistantturn.OutputError {
	return &assistantturn.OutputError{Status: status, Message: fmt.Sprintf(format, args...), Code: "error"}
}
