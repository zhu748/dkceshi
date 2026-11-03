package contextfile

import (
	"context"
	"errors"
	"strings"

	"ds2api/internal/auth"
	dsclient "ds2api/internal/deepseek/client"
	"ds2api/internal/promptcompat"
)

func (s Service) ApplyCurrentInputFile(ctx context.Context, a *auth.RequestAuth, req promptcompat.StandardRequest) (promptcompat.StandardRequest, error) {
	return retryContextFileAccount(ctx, a, req, s.applyCurrentInputFileOnce)
}

func (s Service) ReuploadAppliedCurrentInputFile(ctx context.Context, a *auth.RequestAuth, req promptcompat.StandardRequest) (promptcompat.StandardRequest, error) {
	return retryContextFileAccount(ctx, a, req, s.reuploadAppliedCurrentInputFileOnce)
}

func retryContextFileAccount(ctx context.Context, a *auth.RequestAuth, req promptcompat.StandardRequest, apply func(context.Context, *auth.RequestAuth, promptcompat.StandardRequest) (promptcompat.StandardRequest, error)) (promptcompat.StandardRequest, error) {
	for {
		originalTarget := ""
		if a != nil {
			originalTarget = a.TargetAccount
			if originalTarget == "" {
				a.TargetAccount = a.AccountID
			}
		}
		out, err := apply(ctx, a, req)
		if a != nil {
			a.TargetAccount = originalTarget
		}
		var limited *dsclient.FileRateLimitError
		rateLimited := errors.As(err, &limited)
		if !a.ManagedPool() || (!rateLimited && !dsclient.IsMutedError(err) && !dsclient.IsBannedError(err)) {
			return out, err
		}
		if rateLimited {
			a.NoteRateLimit()
		}
		// Client-provided file IDs have no replayable bytes here. Never carry
		// those references to a new owner, even if they were not explicitly pinned.
		for _, id := range req.RefFileIDs {
			if !strings.EqualFold(id, req.CurrentInputFileID) && !strings.EqualFold(id, req.CurrentToolsFileID) {
				return out, err
			}
		}
		if !dsclient.RetryAccountFailure(ctx, a, err) {
			return out, err
		}
	}
}
