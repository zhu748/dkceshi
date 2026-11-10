package completionruntime

import (
	"context"
	"net/http"

	"ds2api/internal/assistantturn"
	"ds2api/internal/auth"
	"ds2api/internal/chathistory"
	"ds2api/internal/config"
	"ds2api/internal/contextfile"
	dsclient "ds2api/internal/deepseek/client"
	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
	"ds2api/internal/toolcall"
	"ds2api/internal/tooldiagnostics"
)

type DeepSeekCaller interface {
	CreateSession(ctx context.Context, a *auth.RequestAuth, maxAttempts int) (string, error)
	GetPow(ctx context.Context, a *auth.RequestAuth, maxAttempts int) (string, error)
	UploadFile(ctx context.Context, a *auth.RequestAuth, req dsclient.UploadFileRequest, maxAttempts int) (*dsclient.UploadFileResult, error)
	CallCompletion(ctx context.Context, a *auth.RequestAuth, payload map[string]any, powResp string, maxAttempts int) (*http.Response, error)
}

type Options struct {
	StripReferenceMarkers bool
	MaxAttempts           int
	RetryEnabled          bool
	RetryMaxAttempts      int
	CurrentInputFile      contextfile.CurrentInputConfigReader
	ExpertPromptSegment   ExpertPromptSegmentConfigReader
	// ToolCallRepairEnabled turns on the phase-3 finalize-only LLM tool-call
	// repair pass. When enabled, the runtime builds a repair invoker bound to
	// originating model or configured repair API, with an isolated prompt, and
	// hands it to the finalize turn builder so residual bad tool-call code can
	// be repaired. It is never wired into the streaming sieve.
	ToolCallRepairEnabled bool
	// ChatHistory is the response-history store used to record the proactive
	// tool-call repair sub-request as its own "toolcall.repair" entry. Optional:
	// when nil the repair pass runs unrecorded.
	ChatHistory *chathistory.Store
	// toolCallRepair / toolCallRepairCtx are populated internally by the
	// Execute* entrypoints (where ds and auth are available) and threaded into
	// the finalize turn build options.
	toolCallRepair    toolcall.ToolCallRepairInvoker
	toolCallRepairCtx context.Context
	diagnosticCtx     context.Context
}

type NonStreamResult struct {
	SessionID string
	Payload   map[string]any
	Turn      assistantturn.Turn
	Attempts  int
}

type StartResult struct {
	SessionID string
	Payload   map[string]any
	Pow       string
	Response  *http.Response
	Request   promptcompat.StandardRequest
}

func StartCompletion(ctx context.Context, ds DeepSeekCaller, a *auth.RequestAuth, stdReq promptcompat.StandardRequest, opts Options) (StartResult, *assistantturn.OutputError) {
	configureDiagnostics(ctx, a, stdReq)
	start, outErr := startCompletionForRequest(ctx, ds, a, stdReq, opts)
	// 弹性号池/多账号场景下，上游封号（禁言/停用）会让当前账号在开跑阶段直接
	// 失败。此时立即切换到下一个可调度账号重试本次请求，而不是把错误透传给
	// 客户端；被禁言账号的补位在客户端持久化 mute_until 时已同步完成，因此
	// 全程无需人工干预。上游返回的临时 5xx 响应同样在此升级为可重试错误，
	// 否则流式/非流式开跑阶段会把它原样透传给客户端。
	outErr = promoteTransientStartResponse(&start, a, outErr)
	maxAttempts := opts.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	attempted := false
	for outErr != nil && canRetryOnAlternateAccount(ctx, a, outErr, true, &attempted) {
		config.Logger.Info("[completion_runtime_account_switch_retry] retrying start after account failure",
			"surface", stdReq.Surface, "account", a.AccountID, "status", outErr.Status, "error_code", outErr.Code)
		start, outErr = startStandardCompletionOnAlternateAccount(ctx, ds, a, stdReq, opts, maxAttempts)
		outErr = promoteTransientStartResponse(&start, a, outErr)
	}
	return start, outErr
}

// startCompletionForRequest 是 StartCompletion 的纯执行部分：按是否需要分段
// 分派到分段或单次启动，不做账号切换重试。
func startCompletionForRequest(ctx context.Context, ds DeepSeekCaller, a *auth.RequestAuth, stdReq promptcompat.StandardRequest, opts Options) (StartResult, *assistantturn.OutputError) {
	if segments := shouldSegmentExpertPrompt(stdReq, opts); segments != nil {
		return StartCompletionWithSegments(ctx, ds, a, stdReq, opts, segments)
	}
	return startCompletionOnce(ctx, ds, a, stdReq, opts)
}

// PrepareCompletionPayload builds the session-aware completion payload for a
// request without calling the completion endpoint itself. Oversized expert
// prompts are split into segments first (all but the last are sent via
// FireCompletionAndStop), so the caller can stream the returned final payload
// directly. This is the payload-side equivalent of StartCompletion for
// surfaces that stream upstream responses themselves (e.g. the Vercel Node
// stream layer). On success the caller must stream the returned payload with
// the returned PoW on the same account and session.
//
// 与非流式链路一致：账号级鉴权失败（401）先刷新 Token 再换号重试，
// 避免把错误直接透传给客户端（Vercel 流式镜像同样受弹性号池保护）。
func PrepareCompletionPayload(ctx context.Context, ds DeepSeekCaller, a *auth.RequestAuth, stdReq promptcompat.StandardRequest, opts Options, maxAttempts int) (sessionID, pow string, payload map[string]any, outErr *assistantturn.OutputError) {
	start, outErr := PrepareStandardCompletionPayload(ctx, ds, a, stdReq, opts, maxAttempts)
	return start.SessionID, start.Pow, start.Payload, outErr
}

// PrepareStandardCompletionPayload also returns rebuilt input so leases and
// future retries retain the same file owner and prompt as the prepared payload.
func PrepareStandardCompletionPayload(ctx context.Context, ds DeepSeekCaller, a *auth.RequestAuth, stdReq promptcompat.StandardRequest, opts Options, maxAttempts int) (StartResult, *assistantturn.OutputError) {
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	var attempted bool
	start, outErr := prepareCompletionPayloadOnce(ctx, ds, a, stdReq, opts, maxAttempts, false)
	for outErr != nil && canRetryOnAlternateAccount(ctx, a, outErr, true, &attempted) {
		config.Logger.Info("[completion_runtime_account_switch_retry] retrying payload prepare after account failure",
			"surface", stdReq.Surface, "account", a.AccountID, "status", outErr.Status, "error_code", outErr.Code)
		// 换号后当前输入文件必须为新账号重新上传（file_id 是账号级的）。
		start, outErr = prepareCompletionPayloadOnce(ctx, ds, a, stdReq, opts, maxAttempts, true)
	}
	return start, outErr
}

func prepareCompletionPayloadOnce(ctx context.Context, ds DeepSeekCaller, a *auth.RequestAuth, stdReq promptcompat.StandardRequest, opts Options, maxAttempts int, retry bool) (StartResult, *assistantturn.OutputError) {
	var prepErr *assistantturn.OutputError
	if retry {
		stdReq, prepErr = reuploadCurrentInputFileForAccount(ctx, ds, a, stdReq, opts)
	} else {
		stdReq, prepErr = prepareCurrentInputFile(ctx, ds, a, stdReq, opts)
	}
	if prepErr != nil {
		return StartResult{Request: stdReq}, prepErr
	}
	defer pinCompletionAccount(a)()
	sessionID, err := createCompletionSession(ctx, ds, a, maxAttempts, opts.CurrentInputFile)
	if err != nil {
		return StartResult{Request: stdReq}, sessionOutputError(a, err)
	}
	if segments := shouldSegmentExpertPrompt(stdReq, opts); len(segments) > 1 {
		finalPow, finalPayload, segErr := fireSegmentPayloads(ctx, ds, a, stdReq, sessionID, segments, maxAttempts)
		if segErr != nil {
			return StartResult{SessionID: sessionID, Request: stdReq}, segErr
		}
		return StartResult{SessionID: sessionID, Pow: finalPow, Payload: finalPayload, Request: stdReq}, nil
	}
	pow, err := ds.GetPow(ctx, a, maxAttempts)
	if err != nil {
		return StartResult{SessionID: sessionID, Request: stdReq}, powOutputError(a, err)
	}
	return StartResult{SessionID: sessionID, Pow: pow, Payload: stdReq.CompletionPayload(sessionID), Request: stdReq}, nil
}

func startCompletionOnce(ctx context.Context, ds DeepSeekCaller, a *auth.RequestAuth, stdReq promptcompat.StandardRequest, opts Options) (StartResult, *assistantturn.OutputError) {
	maxAttempts := opts.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	var prepErr *assistantturn.OutputError
	stdReq, prepErr = prepareCurrentInputFile(ctx, ds, a, stdReq, opts)
	if prepErr != nil {
		return StartResult{Request: stdReq}, prepErr
	}
	defer pinCompletionAccount(a)()
	sessionID, err := createCompletionSession(ctx, ds, a, maxAttempts, opts.CurrentInputFile)
	if err != nil {
		return StartResult{Request: stdReq}, sessionOutputError(a, err)
	}
	pow, err := ds.GetPow(ctx, a, maxAttempts)
	if err != nil {
		return StartResult{SessionID: sessionID, Request: stdReq}, powOutputError(a, err)
	}
	payload := stdReq.CompletionPayload(sessionID)
	resp, err := ds.CallCompletion(ctx, a, payload, pow, maxAttempts)
	if err != nil {
		return StartResult{SessionID: sessionID, Payload: payload, Pow: pow, Request: stdReq}, completionCallOutputError(ctx, err)
	}
	return StartResult{SessionID: sessionID, Payload: payload, Pow: pow, Response: resp, Request: stdReq}, nil
}

func prepareCurrentInputFile(ctx context.Context, ds DeepSeekCaller, a *auth.RequestAuth, stdReq promptcompat.StandardRequest, opts Options) (promptcompat.StandardRequest, *assistantturn.OutputError) {
	out, err := (contextfile.Service{Store: opts.CurrentInputFile, DS: ds}).ApplyCurrentInputFile(ctx, a, stdReq)
	if err != nil {
		return out, currentInputFileOutputError(err)
	}
	return out, nil
}

// currentInputFileOutputError 把当前输入文件（上传/解析）失败映射为标准输出错误。
// 上传链路同样是账号级操作：Token 被上游拒绝时要带上 account_unauthorized 错误码，
// 让共享重试逻辑先刷新 Token 再换号，而不是把 401 直接透传给客户端。
func currentInputFileOutputError(err error) *assistantturn.OutputError {
	status, message := contextfile.MapError(err)
	code := "error"
	switch {
	case dsclient.IsMutedError(err):
		code = "account_muted"
	case dsclient.IsBannedError(err):
		code = "account_banned"
	case dsclient.IsManagedUnauthorizedError(err):
		code = OutputCodeAccountUnauthorized
	}
	return &assistantturn.OutputError{Status: status, Message: message, Code: code}
}

func ExecuteNonStreamWithRetry(ctx context.Context, ds DeepSeekCaller, a *auth.RequestAuth, stdReq promptcompat.StandardRequest, opts Options) (NonStreamResult, *assistantturn.OutputError) {
	opts = withToolCallRepair(ctx, ds, a, opts, stdReq)
	start, startErr := StartCompletion(ctx, ds, a, stdReq, opts)
	if startErr != nil {
		return NonStreamResult{SessionID: start.SessionID, Payload: start.Payload}, startErr
	}
	return ExecuteNonStreamStartedWithRetry(ctx, ds, a, start, opts)
}

// withToolCallRepair lazily builds the phase-3 repair invoker (bound to the
// request account) and scopes it to ctx, but only when repair is enabled and
// not already configured. The repair pass is finalize-only; the streaming sieve
// must never receive an invoker (plan §5.1).
func withToolCallRepair(ctx context.Context, ds DeepSeekCaller, a *auth.RequestAuth, opts Options, req promptcompat.StandardRequest) Options {
	if !opts.ToolCallRepairEnabled || opts.toolCallRepair != nil {
		return opts
	}
	opts.toolCallRepair = NewToolCallRepairInvoker(ds, a, opts.ChatHistory, req)
	opts.toolCallRepairCtx = ctx
	return opts
}

func ExecuteNonStreamStartedWithRetry(ctx context.Context, ds DeepSeekCaller, a *auth.RequestAuth, start StartResult, opts Options) (finalResult NonStreamResult, finalError *assistantturn.OutputError) {
	opts.diagnosticCtx = ctx
	configureDiagnostics(ctx, a, start.Request)
	defer func() {
		if finalError != nil {
			tooldiagnostics.Failure(ctx, finalError.Code, finalError.Message)
		}
	}()
	opts = withToolCallRepair(ctx, ds, a, opts, start.Request)
	stdReq := start.Request
	maxAttempts := opts.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	sessionID := start.SessionID
	payload := start.Payload
	pow := start.Pow

	attempts := 0
	retries := 0
	accountSwitchAttempted := false
	currentResp := start.Response
	usagePrompt := stdReq.PromptTokenText
	accumulatedRawThinking := ""
	accumulatedToolDetectionThinking := ""
	for {
		turn, outErr := collectAttempt(ctx, currentResp, a, stdReq, usagePrompt, opts)
		if outErr != nil {
			if canRetryOnAlternateAccount(ctx, a, outErr, opts.RetryEnabled, &accountSwitchAttempted) {
				retries++
				switched, switchErr := startStandardCompletionOnAlternateAccount(ctx, ds, a, stdReq, opts, maxAttempts)
				if switchErr != nil {
					return NonStreamResult{SessionID: sessionID, Payload: payload, Turn: turn, Attempts: retries}, switchErr
				}
				if switched.Response != nil {
					config.Logger.Info("[completion_runtime_account_switch_retry] retrying on alternate account", "surface", stdReq.Surface, "stream", false, "account", a.AccountID, "error_code", outErr.Code)
					sessionID = switched.SessionID
					payload = switched.Payload
					pow = switched.Pow
					currentResp = switched.Response
					stdReq = switched.Request
					usagePrompt = stdReq.PromptTokenText
					accumulatedRawThinking = ""
					accumulatedToolDetectionThinking = ""
					continue
				}
			}
			return NonStreamResult{SessionID: sessionID, Payload: payload, Turn: turn, Attempts: retries}, outErr
		}
		accumulatedRawThinking += sse.TrimContinuationOverlap(accumulatedRawThinking, turn.RawThinking)
		accumulatedToolDetectionThinking += sse.TrimContinuationOverlap(accumulatedToolDetectionThinking, turn.DetectionThinking)
		turn.RawThinking = accumulatedRawThinking
		turn.DetectionThinking = accumulatedToolDetectionThinking
		turn = assistantturn.BuildTurnFromCollected(sse.CollectResult{
			Cut:                    turn.ContentCut,
			Text:                   turn.RawText,
			Thinking:               turn.RawThinking,
			ToolDetectionThinking:  turn.DetectionThinking,
			ContentFilter:          turn.ContentFilter,
			UpstreamError:          turn.UpstreamError,
			CitationLinks:          turn.CitationLinks,
			ResponseMessageID:      turn.ResponseMessageID,
			FirstResponseMessageID: turn.FirstResponseMessageID,
		}, buildOptions(stdReq, usagePrompt, opts))

		retryMax := opts.RetryMaxAttempts
		if retryMax <= 0 {
			retryMax = EmptyOutputRetryMaxAttempts()
		}
		toolRetry := IsToolOutputError(turn.Error) && attempts < retryMax
		cooldownEmpty := ObserveAccountOutputFailure(a, turn.Error)
		if cooldownEmpty || !opts.RetryEnabled || !toolRetry && !assistantturn.ShouldRetryEmptyOutput(turn, attempts, retryMax) {
			if canRetryOnAlternateAccount(ctx, a, turn.Error, opts.RetryEnabled, &accountSwitchAttempted) {
				retries++
				switched, switchErr := startStandardCompletionOnAlternateAccount(ctx, ds, a, stdReq, opts, maxAttempts)
				if switchErr != nil {
					return NonStreamResult{SessionID: sessionID, Payload: payload, Turn: turn, Attempts: retries}, switchErr
				}
				if switched.Response != nil {
					config.Logger.Info("[completion_runtime_account_switch_retry] retrying on alternate account", "surface", stdReq.Surface, "stream", false, "account", a.AccountID, "error_code", turn.Error.Code)
					sessionID = switched.SessionID
					payload = switched.Payload
					pow = switched.Pow
					currentResp = switched.Response
					stdReq = switched.Request
					usagePrompt = stdReq.PromptTokenText
					accumulatedRawThinking = ""
					accumulatedToolDetectionThinking = ""
					continue
				}
			}
			if turn.Error != nil {
				reason := "retry_exhausted"
				if !IsRetryableOutputError(turn.Error) {
					reason = "not_retryable"
				}
				if !opts.RetryEnabled {
					reason = "retry_disabled"
				}
				if !IsRetryableOutputError(turn.Error) || !opts.RetryEnabled {
					tooldiagnostics.RetryDecision(ctx, reason, turn.Error.Message)
				}
			}
			return NonStreamResult{SessionID: sessionID, Payload: payload, Turn: turn, Attempts: retries}, turn.Error
		}

		attempts++
		retries++
		diagnosticRetry(ctx, a, "regeneration", turn.Error)
		parentMessageID := retryParentMessageID(turn.ResponseMessageID, payload)
		config.Logger.Info("[completion_runtime_empty_retry] attempting synthetic retry", "surface", stdReq.Surface, "stream", false, "retry_attempt", attempts, "parent_message_id", parentMessageID)
		retryPow, callErr := retryPowForAccount(ctx, ds, a, pow, maxAttempts)
		retryPayload := ClonePayloadForOutputRetry(payload, parentMessageID, turn.Error, ToolRetrySource(turn.RawText, turn.DetectionThinking))
		var nextResp *http.Response
		if callErr == nil {
			var err error
			nextResp, err = ds.CallCompletion(ctx, a, retryPayload, retryPow, maxAttempts)
			if err != nil {
				callErr = completionCallOutputError(ctx, err)
			}
		}
		if callErr != nil {
			if canRetryOnAlternateAccount(ctx, a, callErr, opts.RetryEnabled, &accountSwitchAttempted) {
				retries++
				switched, switchErr := startStandardCompletionOnAlternateAccount(ctx, ds, a, stdReq, opts, maxAttempts)
				if switchErr != nil {
					return NonStreamResult{SessionID: sessionID, Payload: payload, Turn: turn, Attempts: retries}, switchErr
				}
				if switched.Response != nil {
					config.Logger.Info("[completion_runtime_account_switch_retry] retrying after retry request failure", "surface", stdReq.Surface, "stream", false, "account", a.AccountID, "error_code", callErr.Code)
					sessionID = switched.SessionID
					payload = switched.Payload
					pow = switched.Pow
					currentResp = switched.Response
					stdReq = switched.Request
					usagePrompt = stdReq.PromptTokenText
					accumulatedRawThinking = ""
					accumulatedToolDetectionThinking = ""
					continue
				}
			}
			return NonStreamResult{SessionID: sessionID, Payload: payload, Turn: turn, Attempts: retries}, callErr
		}
		payload = retryPayload
		usagePrompt = UsagePromptWithRetryPayload(usagePrompt, retryPayload)
		if toolRetry || isPromptEchoError(turn.Error) {
			accumulatedRawThinking, accumulatedToolDetectionThinking = "", ""
		}
		currentResp = nextResp
	}
}

func retryParentMessageID(observed int, payload map[string]any) int {
	if observed > 0 {
		return observed
	}
	return payloadParentMessageID(payload)
}

func payloadParentMessageID(payload map[string]any) int {
	if payload == nil {
		return 0
	}
	switch v := payload["parent_message_id"].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	default:
		return 0
	}
}

func collectAttempt(ctx context.Context, resp *http.Response, a *auth.RequestAuth, stdReq promptcompat.StandardRequest, usagePrompt string, opts Options) (assistantturn.Turn, *assistantturn.OutputError) {
	if outErr := completionResponseError(resp, a); outErr != nil {
		return assistantturn.Turn{}, outErr
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			config.Logger.Warn("[completion_runtime] response body close failed", "surface", stdReq.Surface, "error", err)
		}
	}()
	result := sse.CollectStreamWithOptions(resp, stdReq.Thinking, false, sse.CollectOptions{Context: ctx}, stdReq.FinalPrompt)
	if result.ReadError != nil {
		// Interrupted output exits before the terminal build. Preserve its
		// partial content, usage and diagnostics without attempting LLM repair.
		turn := assistantturn.BuildTurnFromCollected(result, collectedBuildOptions(ctx, stdReq, usagePrompt, opts, result.ReadError))
		return turn, turn.Error
	}
	// The retry loop combines raw reasoning, then performs the authoritative
	// build once. Parsing, schema validation and BPE tokenization here would
	// immediately be repeated by that build for the same complete response.
	return assistantturn.Turn{
		RawText: result.Text, RawThinking: result.Thinking,
		DetectionThinking: result.ToolDetectionThinking,
		ContentCut:        result.Cut, ContentFilter: result.ContentFilter,
		UpstreamError: result.UpstreamError, CitationLinks: result.CitationLinks,
		ResponseMessageID: result.ResponseMessageID, FirstResponseMessageID: result.FirstResponseMessageID,
	}, nil
}

// OutputCodeAccountUnauthorized 标记"托管账号的 Token 被上游拒绝"。
// 与通用的 "error" 区分开，好让共享重试逻辑知道这是一次账号级鉴权失败，
// 可以刷新 Token / 换号重试，而不是直接把错误透传给客户端。
const OutputCodeAccountUnauthorized = "account_unauthorized"

func authOutputError(a *auth.RequestAuth) *assistantturn.OutputError {
	if a != nil && a.UseConfigToken {
		return &assistantturn.OutputError{Status: http.StatusUnauthorized, Message: "Account token is invalid. Please re-login the account in admin.", Code: OutputCodeAccountUnauthorized}
	}
	return &assistantturn.OutputError{Status: http.StatusUnauthorized, Message: DirectTokenErrorMessage, Code: "error"}
}
