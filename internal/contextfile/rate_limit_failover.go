package contextfile

import (
	"context"
	"errors"

	"ds2api/internal/auth"
	dsclient "ds2api/internal/deepseek/client"
	"ds2api/internal/promptcompat"
)

func (s Service) ApplyCurrentInputFile(ctx context.Context, a *auth.RequestAuth, req promptcompat.StandardRequest) (promptcompat.StandardRequest, error) {
	return retryContextFileAccount(ctx, a, req, s.applyCurrentInputFileOnce)
}

func (s Service) ReuploadAppliedCurrentInputFile(ctx context.Context, a *auth.RequestAuth, req promptcompat.StandardRequest) (promptcompat.StandardRequest, error) {
	return retryContextFileAccount(ctx, a, req, func(ctx context.Context, a *auth.RequestAuth, req promptcompat.StandardRequest) (promptcompat.StandardRequest, error) {
		if !req.CurrentInputFileApplied {
			return s.applyCurrentInputFileOnce(ctx, a, req)
		}
		return s.reuploadAppliedCurrentInputFileOnce(ctx, a, req)
	})
}

func retryContextFileAccount(ctx context.Context, a *auth.RequestAuth, req promptcompat.StandardRequest, apply func(context.Context, *auth.RequestAuth, promptcompat.StandardRequest) (promptcompat.StandardRequest, error)) (promptcompat.StandardRequest, error) {
	for {
		if err := ctx.Err(); err != nil {
			return req, err
		}
		if a != nil && req.ReplayInput != nil && req.InputAccountID != a.AccountID {
			out, err := req.ReplayInput(ctx)
			if err != nil {
				return req, err
			}
			out.ThinkingInjectionPrompt = req.ThinkingInjectionPrompt
			out.FinalPrompt = out.WithThinkingInstruction(out.FinalPrompt)
			out.PromptTokenText = out.FinalPrompt
			req = out
		}
		release := a.PinAccount()
		out, err := apply(ctx, a, req)
		release()
		if err == nil || !a.ManagedPool() {
			return out, err
		}
		var limited *dsclient.FileRateLimitError
		if errors.As(err, &limited) {
			a.NoteRateLimit()
		}
		// Client-provided file IDs have no replayable bytes here. Never carry
		// those references to a new owner, even if they were not explicitly pinned.
		for _, id := range req.RefFileIDs {
			if req.ReplayInput == nil && id != req.CurrentInputFileID && id != req.CurrentToolsFileID {
				return out, err
			}
		}
		if !dsclient.RetryAccountFailure(ctx, a, err) {
			return out, err
		}
	}
}
