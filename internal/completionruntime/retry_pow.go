package completionruntime

import (
	"context"
	"ds2api/internal/assistantturn"
	"ds2api/internal/auth"
	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
)

// GetSessionPow retains the session owner while lower-level authentication is
// refreshed. Only the outer request boundary may rebuild on another account.
func GetSessionPow(ctx context.Context, ds DeepSeekCaller, a *auth.RequestAuth, maxAttempts int) (string, *assistantturn.OutputError) {
	defer a.PinAccount()()
	pow, err := ds.GetPow(ctx, a, maxAttempts)
	if err != nil {
		return "", powOutputError(a, err)
	}
	return pow, nil
}

// Retrying an existing session cannot silently move its PoW to another owner.
// Known account failures must reach the outer rebuild boundary before sending.
func retryPowForAccount(ctx context.Context, ds DeepSeekCaller, a *auth.RequestAuth, previous string, maxAttempts int) (string, *assistantturn.OutputError) {
	defer a.PinAccount()()
	pow, err := ds.GetPow(ctx, a, maxAttempts)
	if err == nil {
		return pow, nil
	}
	if typed := accountFailureOutputError(err); typed != nil {
		return "", typed
	}
	if dsclient.IsManagedUnauthorizedError(err) || dsclient.IsDirectUnauthorizedError(err) || ctx.Err() != nil {
		return "", powOutputError(a, err)
	}
	config.Logger.Warn("[completion_runtime_retry] PoW fetch failed, reusing original", "error", err)
	return previous, nil
}
