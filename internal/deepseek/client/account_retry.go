package client

import (
	"context"
	"errors"

	"ds2api/internal/auth"
)

// RetryAccountFailure is called at boundaries that retain enough input to
// rebuild the whole request. Individual file/session operations stay pinned.
func RetryAccountFailure(ctx context.Context, a *auth.RequestAuth, err error) bool {
	if !a.ManagedPool() || ctx.Err() != nil {
		return false
	}
	var limited *FileRateLimitError
	if errors.As(err, &limited) {
		return a.SwitchAccountOnRateLimit(ctx)
	}
	if IsMutedError(err) || IsBannedError(err) {
		return a.SwitchAccount(ctx)
	}
	if IsManagedUnauthorizedError(err) {
		return a.RecoverAuthFailure(ctx) != auth.AuthRecoveryNone
	}
	return false
}
