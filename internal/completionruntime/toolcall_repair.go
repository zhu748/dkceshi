package completionruntime

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"ds2api/internal/auth"
	"ds2api/internal/chathistory"
	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
	"ds2api/internal/toolcall"
	"ds2api/internal/tooldiagnostics"
)

// toolCallRepairSurface tags the response-history record created for a phase-3
// LLM tool-call repair sub-request, so proactive repair sessions surface in the
// 响应记录 list alongside the main request that triggered them.
const toolCallRepairSurface = "toolcall.repair"

// toolCallRepairModel is only a compatibility fallback for callers without a
// normalized request. Production repair follows the request's resolved model.
const toolCallRepairModel = "deepseek-v4.1-flash"

// toolCallRepairTimeout is the default repair deadline; the admin setting can
// override it. On timeout the invoker returns an error and the caller falls
// back to shared validation, which rejects unresolved tool protocol.
const toolCallRepairTimeout = 10 * time.Second

// toolCallRepairSessionDeleteTimeout bounds the best-effort cleanup delete of a
// repair session. It runs on a context detached from the request/repair
// deadlines (mirroring the autoDeleteRemoteSession mechanism) so the delete is
// not cancelled when the repair itself times out, and does not prolong the main
// request.
const toolCallRepairSessionDeleteTimeout = 10 * time.Second

// repairSessionDeleter is the subset of the DeepSeek client used to reclaim a
// repair session. It is satisfied by *dsclient.Client via a runtime type
// assertion (mirroring the continueResumer pattern in stream_retry.go), so the
// narrow DeepSeekCaller interface stays unchanged.
type repairSessionDeleter interface {
	DeleteSession(ctx context.Context, a *auth.RequestAuth, sessionID string, maxAttempts int) (*dsclient.DeleteSessionResult, error)
}

// NewToolCallRepairInvoker repairs an isolated tool batch using the originating
// resolved model, or the configured OpenAI-compatible API. Request cancellation
// and the configured repair deadline apply to both providers.
func NewToolCallRepairInvoker(ds DeepSeekCaller, a *auth.RequestAuth, history *chathistory.Store, originals ...promptcompat.StandardRequest) toolcall.ToolCallRepairInvoker {
	if ds == nil || a == nil {
		return nil
	}
	originalModel := toolCallRepairModel
	if len(originals) > 0 {
		originalModel = repairModelForRequest(originals[0])
	}
	return func(ctx context.Context, prompt string) (text string, retErr error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		settings := tooldiagnostics.RepairSettingsFromContext(ctx)
		if settings.UnavailableReason != "" {
			return "", errors.New("repair settings unavailable: " + settings.UnavailableReason)
		}
		timeout := toolCallRepairTimeout
		if settings.TimeoutSeconds > 0 {
			timeout = time.Duration(settings.TimeoutSeconds) * time.Second
		}
		repairCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		provider, model := "original", originalModel
		if settings.Mode == "custom" {
			provider, model = "custom_api", settings.Model
		}
		started := time.Now()
		step := tooldiagnostics.Step{RequestAttempt: tooldiagnostics.RepairAttempt(ctx), Stage: "repair_request", Result: "running", Provider: provider, Model: model}
		if provider == "custom_api" {
			step.Protocol = tooldiagnostics.RepairProtocol(settings.Protocol)
		}
		record := startRepairHistory(history, a, prompt, model)
		if record != nil {
			step.HistoryID = record.entryID
		}
		tooldiagnostics.AddStep(ctx, step)
		config.Logger.Info("[toolcall_repair] starting", "provider", provider, "model", model)
		var output repairOutput
		defer func() {
			step.ElapsedMS = time.Since(started).Milliseconds()
			step.Result = "success"
			if retErr != nil {
				step.Result, step.Error = "failed", retErr.Error()
				status := http.StatusBadGateway
				var apiErr *repairAPIError
				if errors.As(retErr, &apiErr) {
					status = apiErr.status
				}
				record.failure(status, retErr)
			} else {
				record.success(output.Text, output.Thinking)
			}
			tooldiagnostics.AddStep(ctx, step)
		}()
		if provider == "custom_api" {
			output, retErr = callRepairAPI(repairCtx, settings, prompt)
		} else {
			output, retErr = callOriginalRepair(repairCtx, ds, a, model, prompt)
		}
		if retErr == nil && repairCtx.Err() != nil {
			output, retErr = repairOutput{}, repairCtx.Err()
		}
		return output.Text, retErr
	}
}

type repairOutput struct{ Text, Thinking string }

func repairModelForRequest(req promptcompat.StandardRequest) string {
	if req.ResolvedModel != "" {
		return req.ResolvedModel
	}
	// ResponseModel may be a compatibility alias; prefer known native IDs when
	// normalized metadata is absent (legacy callers and unit helpers).
	for _, model := range []string{req.ResponseModel, req.RequestedModel} {
		if _, ok := config.GetModelType(model); ok {
			return model
		}
	}
	return toolCallRepairModel
}

func callOriginalRepair(ctx context.Context, ds DeepSeekCaller, a *auth.RequestAuth, model, prompt string) (repairOutput, error) {
	sessionID, err := ds.CreateSession(ctx, a, 1)
	if err != nil {
		return repairOutput{}, err
	}
	defer enqueueRepairSessionDelete(ds, a, sessionID)
	pow, err := ds.GetPow(ctx, a, 1)
	if err != nil {
		return repairOutput{}, err
	}
	std := promptcompat.StandardRequest{Surface: toolCallRepairSurface, ResolvedModel: model, ResponseModel: model, FinalPrompt: prompt, Thinking: true}
	resp, err := ds.CallCompletion(ctx, a, std.CompletionPayload(sessionID), pow, 1)
	if err != nil {
		return repairOutput{}, err
	}
	if resp.StatusCode != http.StatusOK {
		if closeErr := resp.Body.Close(); closeErr != nil {
			config.Logger.Warn("[toolcall_repair] close error response failed", "error", closeErr)
		}
		return repairOutput{}, &repairAPIError{status: resp.StatusCode, message: errToolCallRepairUpstream.Error()}
	}
	result := sse.CollectStreamWithOptions(resp, true, true, sse.CollectOptions{Context: ctx})
	if result.ReadError != nil {
		return repairOutput{}, result.ReadError
	}
	if strings.TrimSpace(result.UpstreamError) != "" {
		return repairOutput{}, errors.New(result.UpstreamError)
	}
	return repairOutput{Text: result.Text, Thinking: result.Thinking}, nil
}

// deleteRepairSession reclaims a repair session on a best-effort basis, wiring
// the phase3 repair pass into the project's session-deletion strategy (the same
// mechanism autoDeleteRemoteSession uses for the main request). It is a no-op
// when the caller cannot delete sessions or the session id is empty. Errors are
// logged, not returned: cleanup failure must never fail the repair.
func deleteRepairSession(parentCtx context.Context, ds DeepSeekCaller, a *auth.RequestAuth, sessionID string) {
	if sessionID == "" {
		return
	}
	deleter, ok := ds.(repairSessionDeleter)
	if !ok {
		return
	}
	// Detach from the parent (and the already-expired repair) context so the
	// delete is not cancelled by a repair timeout or a completed request.
	deleteCtx, cancel := context.WithTimeout(context.WithoutCancel(parentCtx), toolCallRepairSessionDeleteTimeout)
	defer cancel()
	if _, err := deleter.DeleteSession(deleteCtx, a, sessionID, 1); err != nil {
		config.Logger.Warn("[toolcall_repair] failed to delete repair session", "session_id", sessionID, "error", err)
	}
}

var errToolCallRepairUpstream = errors.New("toolcall repair: upstream returned non-200")
