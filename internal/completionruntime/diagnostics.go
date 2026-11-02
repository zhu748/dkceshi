package completionruntime

import (
	"context"

	"ds2api/internal/assistantturn"
	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
	"ds2api/internal/tooldiagnostics"
)

func configureDiagnostics(ctx context.Context, a *auth.RequestAuth, req promptcompat.StandardRequest) {
	if a == nil {
		return
	}
	tooldiagnostics.Configure(ctx, req.ResponseModel, a.CallerID, a.AccountID, req.Stream, req.ToolsRaw, req.ToolChoice)
}

func diagnosticRetry(ctx context.Context, a *auth.RequestAuth, kind string, err *assistantturn.OutputError) {
	if err == nil {
		return
	}
	account := ""
	if a != nil {
		account = a.AccountID
	}
	tooldiagnostics.NextAttempt(ctx, kind, err.Code, err.Message, account)
}

func recordStreamFailure(ctx context.Context, req promptcompat.StandardRequest, prompt string, hooks StreamRetryHooks, failure *assistantturn.OutputError) {
	if tooldiagnostics.FromContext(ctx) == nil {
		return
	}
	if failure == nil && hooks.EmptyOutputError != nil {
		failure = hooks.EmptyOutputError()
	}
	if ctx.Err() != nil {
		failure = &assistantturn.OutputError{Status: 499, Code: "request_cancelled", Message: ctx.Err().Error()}
	}
	if failure == nil {
		return
	}
	if hooks.DiagnosticOutput == nil {
		tooldiagnostics.Failure(ctx, failure.Code, failure.Message)
		return
	}
	text, thinking, detection := hooks.DiagnosticOutput()
	opts := buildOptionsWithoutRepair(req, prompt, Options{})
	opts.DiagnosticContext = ctx
	assistantturn.ObserveFailedOutput(text, thinking, detection, opts, failure)
}
