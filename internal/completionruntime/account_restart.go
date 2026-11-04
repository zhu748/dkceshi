package completionruntime

import (
	"context"
	"ds2api/internal/assistantturn"
	"ds2api/internal/auth"
	"ds2api/internal/contextfile"
	"ds2api/internal/promptcompat"
	"net/http"
)

func canRetryOnAlternateAccount(ctx context.Context, a *auth.RequestAuth, outErr *assistantturn.OutputError, retryEnabled bool, attempted *bool) bool {
	if outErr == nil || !retryEnabled || a == nil || !a.UseConfigToken {
		return false
	}
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	if outErr.Status == http.StatusTooManyRequests && outErr.Code != "upstream_empty_output" && a.ManagedPool() {
		return a.SwitchAccountOnRateLimit(ctx)
	}
	if assistantturn.IsUnavailableEmptyOutput(outErr) && a.ManagedPool() {
		return a.SwitchAccountOnRateLimit(ctx)
	}
	if isAccountMuted(outErr) {
		return a.SwitchAccount(ctx)
	}
	if isUpstreamUnavailable(outErr) {
		return a.SwitchAccount(ctx)
	}
	if isRetryableUpstreamError(outErr) {
		return a.SwitchAccount(ctx)
	}
	if isManagedUnauthorized(outErr) {
		// 鉴权失败的恢复顺序（先刷新、再换号）由 auth 层共享，与 client 层的上游
		// 调用点和 Vercel 路径同源，见 auth.RequestAuth.RecoverAuthFailure。
		return a.RecoverAuthFailure(ctx) != auth.AuthRecoveryNone
	}
	if outErr.Status != http.StatusTooManyRequests && !IsToolOutputError(outErr) {
		return false
	}
	if attempted == nil || *attempted {
		return false
	}
	*attempted = true
	return a.SwitchAccount(ctx)
}

// isManagedUnauthorized 判断错误是否表示托管账号的 Token 被上游拒绝。
// 只认显式的 account_unauthorized 错误码：所有托管账号的 401 都会在构造输出
// 错误时归一化为该错误码（见 authOutputError / sessionOutputError /
// powOutputError / managedUnauthorizedResponseError）。若再用 HTTP 401 兜底，
// 就会把网络抖动一类的失败也当成账号级鉴权失败，触发无谓的强制重新登录与换号。
func isManagedUnauthorized(outErr *assistantturn.OutputError) bool {
	return outErr != nil && outErr.Code == OutputCodeAccountUnauthorized
}

func isUpstreamUnavailable(outErr *assistantturn.OutputError) bool {
	return outErr != nil && outErr.Code == "upstream_unavailable"
}

func isAccountMuted(outErr *assistantturn.OutputError) bool {
	return outErr != nil && (outErr.Code == "account_muted" || outErr.Code == "account_banned")
}

func startStandardCompletionOnAlternateAccount(ctx context.Context, ds DeepSeekCaller, a *auth.RequestAuth, stdReq promptcompat.StandardRequest, opts Options, maxAttempts int) (StartResult, *assistantturn.OutputError) {
	opts.MaxAttempts = maxAttempts
	attempted := false
	for {
		start, outErr := restartStandardCompletion(ctx, ds, a, stdReq, opts)
		if outErr == nil {
			outErr = completionResponseError(start.Response, a)
		}
		if outErr == nil || !canRetryOnAlternateAccount(ctx, a, outErr, opts.RetryEnabled, &attempted) {
			return start, outErr
		}
		stdReq = start.Request
	}
}

func restartStandardCompletion(ctx context.Context, ds DeepSeekCaller, a *auth.RequestAuth, req promptcompat.StandardRequest, opts Options) (StartResult, *assistantturn.OutputError) {
	rebuilt, outErr := reuploadCurrentInputFileForAccount(ctx, ds, a, req, opts)
	if outErr != nil {
		return StartResult{Request: rebuilt}, outErr
	}
	return startCompletionForRequest(ctx, ds, a, rebuilt, opts)
}

func reuploadCurrentInputFileForAccount(ctx context.Context, ds DeepSeekCaller, a *auth.RequestAuth, stdReq promptcompat.StandardRequest, opts Options) (promptcompat.StandardRequest, *assistantturn.OutputError) {
	out, err := (contextfile.Service{Store: opts.CurrentInputFile, DS: ds}).ReuploadAppliedCurrentInputFile(ctx, a, stdReq)
	if err != nil {
		return out, currentInputFileOutputError(err)
	}
	return out, nil
}
