package toolcall

import (
	"context"
	"errors"
	"time"

	"ds2api/internal/tooldiagnostics"
)

// Retrying the complete format operation includes both transport failures and
// invalid model output. One overall deadline bounds all calls and backoff.
func RepairToolCallsWithLLM(ctx context.Context, badCode string, invoke ToolCallRepairInvoker, toolMarker ...string) ([]ParsedToolCall, bool) {
	if ctx.Err() != nil {
		return nil, false
	}
	settings := tooldiagnostics.RepairSettingsFromContext(ctx)
	if settings.UnavailableReason != "" {
		// Configuration faults disable model calls, not deterministic recovery.
		calls, ok := repairToolCallsOnce(ctx, badCode, nil, toolMarker...)
		if !ok {
			tooldiagnostics.AddStep(ctx, tooldiagnostics.Step{Stage: "llm_repair", Result: "skipped", Reason: "repair_settings_unavailable", Error: settings.UnavailableReason})
		}
		return calls, ok
	}
	retries := 0
	if settings.Mode == "custom" {
		retries = min(10, max(0, tooldiagnostics.RepairRetryCount(settings)))
		timeout := settings.TimeoutSeconds
		if timeout <= 0 {
			timeout = 10
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()
	}
	for attempt := 0; attempt <= retries; attempt++ {
		if ctx.Err() != nil {
			return nil, false
		}
		called := false
		var invokeError error
		var wrapped ToolCallRepairInvoker
		if invoke != nil {
			wrapped = func(ctx context.Context, prompt string) (string, error) {
				called = true
				output, err := invoke(ctx, prompt)
				invokeError = err
				return output, err
			}
		}
		attemptCtx := tooldiagnostics.WithRepairAttempt(ctx, attempt+1)
		calls, ok := repairToolCallsOnce(attemptCtx, badCode, wrapped, toolMarker...)
		if ctx.Err() != nil {
			return nil, false
		}
		if ok || !called || attempt == retries {
			return calls, ok
		}
		var policy interface{ Retryable() bool }
		if errors.As(invokeError, &policy) && !policy.Retryable() {
			return nil, false
		}
		delay := time.Duration(250*(1<<min(attempt, 4))) * time.Millisecond
		var timing interface{ RetryDelay() time.Duration }
		if errors.As(invokeError, &timing) {
			delay = max(delay, timing.RetryDelay())
		}
		tooldiagnostics.AddStep(attemptCtx, tooldiagnostics.Step{Stage: "repair_retry", Result: "started", RequestAttempt: attempt + 2, Reason: "custom_api_retry"})
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, false
		case <-timer.C:
		}
	}
	return nil, false
}
