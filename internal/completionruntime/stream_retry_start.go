package completionruntime

import (
	"context"
	"io"
	"net/http"

	"ds2api/internal/assistantturn"
	"ds2api/internal/auth"
	"ds2api/internal/config"
)

func startPayloadCompletionOnAlternateAccount(ctx context.Context, ds DeepSeekCaller, a *auth.RequestAuth, payload map[string]any, opts StreamRetryOptions, maxAttempts int) (StartResult, *assistantturn.OutputError) {
	attempted := false
	for {
		start, outErr := startPayloadCompletionOnce(ctx, ds, a, payload, opts, maxAttempts)
		if outErr == nil {
			outErr = completionResponseError(start.Response, a)
		}
		if outErr == nil {
			return start, nil
		}
		if !canRetryOnAlternateAccount(ctx, a, outErr, opts.RetryEnabled, &attempted) {
			return start, outErr
		}
	}
}

func startPayloadCompletionOnce(ctx context.Context, ds DeepSeekCaller, a *auth.RequestAuth, payload map[string]any, opts StreamRetryOptions, maxAttempts int) (StartResult, *assistantturn.OutputError) {
	if segments := shouldSegmentExpertPrompt(opts.Request, Options{ExpertPromptSegment: opts.ExpertPromptSegment}); segments != nil {
		return StartCompletionWithSegments(ctx, ds, a, opts.Request, Options{
			MaxAttempts:         maxAttempts,
			RetryEnabled:        opts.RetryEnabled,
			RetryMaxAttempts:    opts.RetryMaxAttempts,
			CurrentInputFile:    opts.CurrentInputFile,
			ExpertPromptSegment: opts.ExpertPromptSegment,
		}, segments)
	}
	sessionID, err := ds.CreateSession(ctx, a, maxAttempts)
	if err != nil {
		return StartResult{}, sessionOutputError(a, err)
	}
	pow, err := ds.GetPow(ctx, a, maxAttempts)
	if err != nil {
		return StartResult{SessionID: sessionID}, powOutputError(a, err)
	}
	nextPayload := clonePayload(payload)
	if opts.CurrentInputFile != nil && opts.Request.CurrentInputFileApplied {
		stdReq, prepErr := reuploadCurrentInputFileForAccount(ctx, ds, a, opts.Request, Options{CurrentInputFile: opts.CurrentInputFile})
		if prepErr != nil {
			return StartResult{SessionID: sessionID}, prepErr
		}
		nextPayload = stdReq.CompletionPayload(sessionID)
	}
	nextPayload["chat_session_id"] = sessionID
	delete(nextPayload, "parent_message_id")
	resp, err := ds.CallCompletion(ctx, a, nextPayload, pow, maxAttempts)
	if err != nil {
		return StartResult{SessionID: sessionID, Payload: nextPayload, Pow: pow}, completionCallOutputError(ctx, err)
	}
	return StartResult{SessionID: sessionID, Payload: nextPayload, Pow: pow, Response: resp}, nil
}

func buildEmptyOutputError(hooks StreamRetryHooks) *assistantturn.OutputError {
	if hooks.EmptyOutputError != nil {
		if err := hooks.EmptyOutputError(); err != nil {
			return err
		}
	}
	return &assistantturn.OutputError{Status: http.StatusTooManyRequests, Code: "upstream_empty_output"}
}

func clonePayload(payload map[string]any) map[string]any {
	clone := make(map[string]any, len(payload))
	for k, v := range payload {
		clone[k] = v
	}
	return clone
}

func closeRetryBody(surface string, body io.Closer) {
	if body == nil {
		return
	}
	if err := body.Close(); err != nil {
		config.Logger.Warn("[completion_runtime_empty_retry] retry response body close failed", "surface", surface, "error", err)
	}
}
