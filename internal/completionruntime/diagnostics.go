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
