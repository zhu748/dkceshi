package completionruntime

import (
	"context"
	"net/http"
	"strings"

	"ds2api/internal/assistantturn"
	"ds2api/internal/auth"
	"ds2api/internal/config"
	"ds2api/internal/httpapi/openai/history"
	"ds2api/internal/httpapi/openai/shared"
	"ds2api/internal/promptcompat"
)

type StreamRetryOptions struct {
	Surface             string
	Stream              bool
	RetryEnabled        bool
	RetryMaxAttempts    int
	MaxAttempts         int
	UsagePrompt         string
	Request             promptcompat.StandardRequest
	CurrentInputFile    history.CurrentInputConfigReader
	ExpertPromptSegment ExpertPromptSegmentConfigReader
}

// ConsumeAttemptResult describes what one stream-consuming attempt ended with.
type ConsumeAttemptResult struct {
	// Terminal indicates the attempt wrote its terminal frame(s) (finish
	// chunk, error chunk or [DONE]); the retry loop must stop.
	Terminal bool
	// Retryable indicates the attempt produced no usable output and the loop
	// may retry with a synthetic empty-output payload.
	Retryable bool
	// ResumeContinue indicates the upstream stream was interrupted after
	// producing partial output; the loop should resume the same message via
	// the continue endpoint instead of starting an empty-output retry.
	ResumeContinue bool
}

type StreamRetryHooks struct {
	ConsumeAttempt  func(resp *http.Response, allowDeferEmpty bool) ConsumeAttemptResult
	Finalize        func(attempts int)
	ParentMessageID func() int
	OnRetry         func(attempts int)
	// OnRegenerate discards undelivered output before a new generation.
	// Continue/resume deliberately preserves the current attempt state.
	OnRegenerate     func()
	OnRetryPrompt    func(prompt string)
	OnRetryFailure   func(status int, message, code string)
	OnAccountSwitch  func(sessionID string)
	OnTerminal       func(attempts int)
	EmptyOutputError func() *assistantturn.OutputError
}

// continueResumer is implemented by DeepSeek callers that can resume a
// partially generated message through the upstream continue endpoint.
type continueResumer interface {
	ContinueCompletion(ctx context.Context, a *auth.RequestAuth, sessionID string, messageID int, powResp string) (*http.Response, error)
}

func ExecuteStreamWithRetry(ctx context.Context, ds DeepSeekCaller, a *auth.RequestAuth, initialResp *http.Response, payload map[string]any, pow string, opts StreamRetryOptions, hooks StreamRetryHooks) {
	if hooks.ConsumeAttempt == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	surface := strings.TrimSpace(opts.Surface)
	if surface == "" {
		surface = "completion"
	}
	maxAttempts := opts.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	retryMax := opts.RetryMaxAttempts
	if retryMax <= 0 {
		retryMax = shared.EmptyOutputRetryMaxAttempts()
	}
	attempts := 0
	accountSwitchAttempted := false
	currentResp := initialResp
	currentPayload := clonePayload(payload)
	failed := false
	fail := func(err *assistantturn.OutputError) {
		if failed {
			return
		}
		failed = true
		if ctx.Err() == nil && hooks.OnRetryFailure != nil {
			hooks.OnRetryFailure(err.Status, err.Message, err.Code)
		}
	}
	reset := func() {
		if hooks.OnRegenerate != nil {
			hooks.OnRegenerate()
		}
	}
	adopt := func(start StartResult) {
		currentResp, currentPayload, pow = start.Response, start.Payload, start.Pow
		reset()
		if hooks.OnAccountSwitch != nil {
			hooks.OnAccountSwitch(start.SessionID)
		}
		if hooks.OnRetryPrompt != nil {
			hooks.OnRetryPrompt(opts.UsagePrompt)
		}
	}
	switchAccount := func(err *assistantturn.OutputError) bool {
		if !canRetryOnAlternateAccount(ctx, a, err, opts.RetryEnabled, &accountSwitchAttempted) {
			return false
		}
		start, startErr := startPayloadCompletionOnAlternateAccount(ctx, ds, a, payload, opts, maxAttempts)
		if startErr != nil {
			fail(startErr)
			return false
		}
		adopt(start)
		config.Logger.Info("[completion_runtime_account_switch_retry] retrying on alternate account", "surface", surface, "account", a.AccountID, "error_code", err.Code)
		return true
	}
	for {
		if ctx.Err() != nil {
			if currentResp != nil && currentResp.Body != nil {
				closeRetryBody(surface, currentResp.Body)
			}
			return
		}
		if responseErr := completionResponseError(currentResp, a); responseErr != nil {
			if switchAccount(responseErr) {
				continue
			}
			fail(responseErr)
			return
		}
		allowDefer := opts.RetryEnabled && (attempts < retryMax || a != nil && a.UseConfigToken)
		result := hooks.ConsumeAttempt(currentResp, allowDefer)
		if ctx.Err() != nil {
			return
		}
		if result.Terminal {
			if hooks.OnTerminal != nil {
				hooks.OnTerminal(attempts)
			}
			return
		}
		pendingErr := buildEmptyOutputError(hooks)
		if !result.Retryable || !opts.RetryEnabled || !result.ResumeContinue && !IsRetryableOutputError(pendingErr) {
			if !failed && hooks.Finalize != nil {
				hooks.Finalize(attempts)
			}
			return
		}
		if attempts >= retryMax {
			if result.ResumeContinue {
				fail(&assistantturn.OutputError{Status: http.StatusBadGateway, Message: "Upstream stream interrupted and continue-resume attempts are exhausted.", Code: "upstream_interrupted"})
				return
			}
			if switchAccount(pendingErr) {
				continue
			}
			if !failed && hooks.EmptyOutputError != nil && hooks.EmptyOutputError() != nil {
				// Finalization already validated and optionally repaired this
				// attempt. Report the stored failure without running repair twice.
				fail(pendingErr)
			} else if !failed && hooks.Finalize != nil {
				hooks.Finalize(attempts)
			}
			return
		}
		attempts++
		parentMessageID := 0
		if hooks.ParentMessageID != nil {
			parentMessageID = hooks.ParentMessageID()
		}
		parentMessageID = retryParentMessageID(parentMessageID, currentPayload)
		retryPow, powErr := ds.GetPow(ctx, a, maxAttempts)
		if powErr != nil {
			config.Logger.Warn("[completion_runtime_retry] PoW fetch failed, reusing original", "surface", surface, "error", powErr)
			retryPow = pow
		}
		if result.ResumeContinue {
			sessionID, _ := currentPayload["chat_session_id"].(string)
			next, err := resumeStream(ctx, ds, a, sessionID, parentMessageID, retryPow)
			if err != nil {
				fail(err)
				return
			}
			currentResp = next
			if hooks.OnRetry != nil {
				hooks.OnRetry(attempts)
			}
			continue
		}
		config.Logger.Info("[completion_runtime_output_retry] regenerating undelivered output", "surface", surface, "retry_attempt", attempts, "error_code", pendingErr.Code)
		retryPayload := shared.ClonePayloadForEmptyOutputRetry(currentPayload, parentMessageID)
		if IsToolOutputError(pendingErr) {
			retryPayload = shared.ClonePayloadForToolOutputRetry(currentPayload, parentMessageID)
		}
		next, callErr := ds.CallCompletion(ctx, a, retryPayload, retryPow, maxAttempts)
		var outErr *assistantturn.OutputError
		if callErr != nil {
			outErr = completionCallOutputError(ctx, callErr)
		} else {
			outErr = completionResponseError(next, a)
		}
		if outErr != nil {
			if switchAccount(outErr) {
				continue
			}
			fail(outErr)
			return
		}
		reset()
		if hooks.OnRetry != nil {
			hooks.OnRetry(attempts)
		}
		if hooks.OnRetryPrompt != nil {
			usagePrompt := shared.UsagePromptWithEmptyOutputRetry(opts.UsagePrompt, attempts)
			if IsToolOutputError(pendingErr) {
				usagePrompt = shared.UsagePromptWithToolOutputRetry(opts.UsagePrompt, attempts)
			}
			hooks.OnRetryPrompt(usagePrompt)
		}
		currentResp, currentPayload = next, retryPayload
	}
}
