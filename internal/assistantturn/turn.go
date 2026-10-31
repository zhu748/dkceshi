package assistantturn

import (
	"context"
	"net/http"
	"strings"

	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
	"ds2api/internal/textclean"
	"ds2api/internal/toolcall"
	"ds2api/internal/util"
)

type StopReason string

const (
	StopReasonStop          StopReason = "stop"
	StopReasonToolCalls     StopReason = "tool_calls"
	StopReasonContentFilter StopReason = "content_filter"
	StopReasonError         StopReason = "error"
)

type Usage struct {
	InputTokens     int
	OutputTokens    int
	ReasoningTokens int
	TotalTokens     int
}

type OutputError struct {
	Status  int
	Message string
	Code    string
}

type Turn struct {
	Model             string
	Prompt            string
	RawText           string
	RawThinking       string
	DetectionThinking string
	Text              string
	Thinking          string
	ToolCalls         []toolcall.ParsedToolCall
	ParsedToolCalls   toolcall.ToolCallParseResult
	CitationLinks     map[int]string
	ContentFilter     bool
	UpstreamError     string
	ResponseMessageID int
	// FirstResponseMessageID is the id the completion started with. It differs
	// from ResponseMessageID when auto-continue appended segments; TTS reads
	// the original assistant message and must use this one.
	FirstResponseMessageID int
	StopReason             StopReason
	Usage                  Usage
	Error                  *OutputError
}

type FinalizeOptions struct {
	AlreadyEmittedToolCalls bool
}

type FinalOutcome struct {
	FinishReason     string
	Error            *OutputError
	Usage            Usage
	HasToolCalls     bool
	HasVisibleText   bool
	HasVisibleOutput bool
	ShouldFail       bool
}

type BuildOptions struct {
	Model                 string
	Prompt                string
	RefFileTokens         int
	SearchEnabled         bool
	StripReferenceMarkers bool
	ToolNames             []string
	ToolsRaw              any
	ToolChoice            promptcompat.ToolChoicePolicy
	// ToolCallRepair, when non-nil, enables the phase-3 LLM tool-call repair
	// pass in the finalize path: if the parsed result found a residual tool
	// call intent, the complete source batch is handed to this
	// invoker for repair. It must never be wired into the per-chunk streaming
	// sieve (plan/tool-call-fallback-design-phase3.md §5.1).
	ToolCallRepair toolcall.ToolCallRepairInvoker
	// ToolCallRepairCtx scopes the repair invocation; defaults to
	// context.Background() when nil.
	ToolCallRepairCtx context.Context
	// ToolMarker is the caller-specific tool-call marker used by the model.
	// Raw model output is normalized from ToolMarker back to EPSE before
	// parsing/repair so the internal tool-call model only ever sees EPSE.
	ToolMarker string
	// PromptTokenCountFn, when non-nil, supplies the prompt-side token count
	// for BuildUsage. Streaming runtimes already count the same prompt text for
	// their first frame (e.g. Claude's message_start); reusing that count here
	// avoids a second full BPE encode of a potentially hundreds-of-KB prompt.
	// ok=false falls back to counting from Prompt. RefFileTokens is still added
	// on top of whatever this returns.
	PromptTokenCountFn func(prompt string) (count int, ok bool)
}

// maybeRepairToolCalls runs only at finalization when syntax or residual intent
// needs recovery. A successful repair replaces the whole ordered source batch;
// format-only integrity checks preserve every named occurrence and argument.
// The returned residual identifies visible bad markup that can be removed.
func maybeRepairToolCalls(parsed toolcall.ToolCallParseResult, rawText string, opts BuildOptions) ([]toolcall.ParsedToolCall, string) {
	badCode := strings.TrimSpace(parsed.ResidualIntentText)
	replaceable := parsed.SawToolCallIntent && badCode != ""
	if !parsed.SawToolCallIntent {
		// Direction 4 defensive path: intent detection can miss a residual
		// (e.g. odd markdown fences confusing the residual probe), but the
		// deterministic parser still saw tool-call syntax and produced zero
		// calls. Rather than silently leaking the bad tool-call code as visible
		// text, hand the raw (un-stripped) text to the repair layer. Only fires
		// when syntax was seen AND nothing parsed, so well-formed / partial
		// successes are unaffected.
		if !parsed.SawToolCallSyntax || len(parsed.Calls) > 0 {
			return nil, ""
		}
		badCode = strings.TrimSpace(rawText)
		// The defensive path hands the whole raw text to the model; it is not a
		// sourced-out residual region, so it must not be deducted from visible
		// text.
		replaceable = false
	}
	// Repair the whole source batch, including already parsed siblings. The
	// repair integrity manifest binds every occurrence to its original order;
	// appending residual calls or value-based deduplication cannot do that.
	if parsed.SourceText != "" {
		badCode = parsed.SourceText
	}
	if badCode == "" {
		return nil, ""
	}
	ctx := opts.ToolCallRepairCtx
	if ctx == nil {
		ctx = context.Background()
	}
	calls, ok := toolcall.RepairToolCallsWithLLM(ctx, badCode, opts.ToolCallRepair, opts.ToolMarker)
	if !ok || len(calls) == 0 {
		return nil, ""
	}
	normalized := toolcall.NormalizeParsedToolCallsForSchemas(calls, opts.ToolsRaw)
	if !replaceable {
		return normalized, ""
	}
	return normalized, parsed.ResidualIntentText
}

// deductRepairedSource removes the bad tool-markup regions that a successful
// LLM repair replaced from the visible text.
//
// Removal is span based rather than verbatim-residual based. The residual
// handed to the repair layer is not necessarily a contiguous substring of the
// visible text (a successfully parsed wrapper is cut out of its middle, leaving
// head and tail) and it can carry legitimate prose next to the bad call, so
// only the markup spans themselves are deducted and the prose survives. When no
// markup span is locatable (for example an EPSE shell whose local name is
// outside the markup name table), the residual is removed verbatim as a
// fallback, which preserves the previous behaviour for contiguous residuals.
func deductRepairedSource(text, residual string) string {
	if text == "" {
		return text
	}
	if spans := toolcall.BadToolMarkupSpans(text); len(spans) > 0 {
		return toolcall.RemoveTextSpans(text, spans)
	}
	replaced := strings.TrimSpace(residual)
	if replaced == "" {
		return text
	}
	idx := strings.Index(text, replaced)
	if idx < 0 {
		return text
	}
	return strings.TrimSpace(text[:idx] + text[idx+len(replaced):])
}

type StreamSnapshot struct {
	RawText               string
	VisibleText           string
	RawThinking           string
	VisibleThinking       string
	DetectionThinking     string
	ContentFilter         bool
	UpstreamError         string
	CitationLinks         map[int]string
	ResponseMessageID     int
	AlreadyEmittedCalls   bool
	AdditionalToolCalls   []toolcall.ParsedToolCall
	AlreadyEmittedToolRaw bool
}

func BuildTurnFromCollected(result sse.CollectResult, opts BuildOptions) Turn {
	result.Text = toolcall.NormalizeMarkerText(result.Text, opts.ToolMarker)
	result.Thinking = toolcall.NormalizeMarkerText(result.Thinking, opts.ToolMarker)
	result.ToolDetectionThinking = toolcall.NormalizeMarkerText(result.ToolDetectionThinking, opts.ToolMarker)
	// Shared final safety net: the collectors already cut a hallucinated
	// `[Tool]:` role marker on the raw text channel, and this keeps the
	// invariant for any path that builds a turn without going through them —
	// the model's invented tool result and the monologue after it are never
	// delivered as reply text.
	result.Text, _ = sse.TrimToolRoleMarker(result.Text)
	thinking := textclean.CleanVisibleOutput(result.Thinking, opts.StripReferenceMarkers)
	text := textclean.CleanVisibleOutput(result.Text, opts.StripReferenceMarkers)
	text = toolcall.StripDSMLResidue(text)
	if opts.SearchEnabled {
		text = textclean.ReplaceCitationMarkersWithLinks(text, result.CitationLinks)
	}

	parsed := DetectAssistantToolCalls(result.Text, text, result.Thinking, result.ToolDetectionThinking, opts.ToolNames)
	calls := toolcall.NormalizeParsedToolCallsForSchemas(parsed.Calls, opts.ToolsRaw)
	repairCompleted := false
	if result.ReadError == nil {
		if repaired, replaced := maybeRepairToolCalls(parsed, result.Text, opts); len(repaired) > 0 {
			calls = repaired
			text = deductRepairedSource(text, replaced)
			repairCompleted = true
		}
	}
	parsed.Calls = calls

	stopReason := StopReasonStop
	if result.ContentFilter {
		stopReason = StopReasonContentFilter
	}
	if len(calls) > 0 {
		stopReason = StopReasonToolCalls
	}

	turn := Turn{
		Model:                  opts.Model,
		Prompt:                 opts.Prompt,
		RawText:                result.Text,
		RawThinking:            result.Thinking,
		DetectionThinking:      result.ToolDetectionThinking,
		Text:                   text,
		Thinking:               thinking,
		ToolCalls:              calls,
		ParsedToolCalls:        parsed,
		CitationLinks:          result.CitationLinks,
		ContentFilter:          result.ContentFilter,
		UpstreamError:          result.UpstreamError,
		ResponseMessageID:      result.ResponseMessageID,
		FirstResponseMessageID: result.FirstResponseMessageID,
		StopReason:             stopReason,
	}
	turn.Usage = buildUsageWithPrefill(opts, thinking, text)
	turn.Error = validateToolOutput(turn, opts, repairCompleted)
	if result.ReadError != nil {
		turn.Error = &OutputError{Status: http.StatusBadGateway, Code: "upstream_read_error", Message: "Upstream stream read failed: " + result.ReadError.Error()}
	}
	if turn.Error != nil {
		turn.StopReason = StopReasonError
	}
	return turn
}

func BuildTurnFromStreamSnapshot(snapshot StreamSnapshot, opts BuildOptions) Turn {
	snapshot.RawText = toolcall.NormalizeMarkerText(snapshot.RawText, opts.ToolMarker)
	snapshot.RawThinking = toolcall.NormalizeMarkerText(snapshot.RawThinking, opts.ToolMarker)
	snapshot.DetectionThinking = toolcall.NormalizeMarkerText(snapshot.DetectionThinking, opts.ToolMarker)
	// Shared final safety net: the stream runtimes already cut a hallucinated
	// `[Tool]:` role marker on the raw text channel (sse.ToolRoleMarkerGuard),
	// and this keeps the invariant for any path that builds a turn without
	// going through them — neither the invented tool result nor the monologue
	// after it can reach the reply text or the finalize tool detection.
	snapshot.RawText, _ = sse.TrimToolRoleMarker(snapshot.RawText)
	snapshot.VisibleText, _ = sse.TrimToolRoleMarker(snapshot.VisibleText)
	thinking := textclean.CleanVisibleOutput(snapshot.VisibleThinking, opts.StripReferenceMarkers)
	text := textclean.CleanVisibleOutput(snapshot.VisibleText, opts.StripReferenceMarkers)
	text = toolcall.StripDSMLResidue(text)
	if opts.SearchEnabled {
		text = textclean.ReplaceCitationMarkersWithLinks(text, snapshot.CitationLinks)
	}

	parsed := DetectAssistantToolCalls(snapshot.RawText, text, snapshot.RawThinking, snapshot.DetectionThinking, opts.ToolNames)
	calls := parsed.Calls
	if len(calls) == 0 && len(snapshot.AdditionalToolCalls) > 0 {
		calls = snapshot.AdditionalToolCalls
	}
	calls = toolcall.NormalizeParsedToolCallsForSchemas(calls, opts.ToolsRaw)
	repairCompleted := false
	// M4/M3: run the finalize LLM repair before delivering the final frame,
	// unless structured tool calls were already streamed to the client (we
	// cannot retract emitted tool_calls). When only bad text was streamed, the
	// finalize replaces it with the repaired tool_calls. Partial success is
	// repaired as one ordered batch without value-based deduplication.
	if !snapshot.AlreadyEmittedCalls && !snapshot.AlreadyEmittedToolRaw {
		if repaired, replaced := maybeRepairToolCalls(parsed, snapshot.RawText, opts); len(repaired) > 0 {
			calls = repaired
			text = deductRepairedSource(text, replaced)
			repairCompleted = true
		}
	}
	parsed.Calls = calls

	stopReason := StopReasonStop
	if snapshot.ContentFilter {
		stopReason = StopReasonContentFilter
	}
	if len(calls) > 0 || snapshot.AlreadyEmittedCalls || snapshot.AlreadyEmittedToolRaw {
		stopReason = StopReasonToolCalls
	}

	turn := Turn{
		Model:             opts.Model,
		Prompt:            opts.Prompt,
		RawText:           snapshot.RawText,
		RawThinking:       snapshot.RawThinking,
		DetectionThinking: snapshot.DetectionThinking,
		Text:              text,
		Thinking:          thinking,
		ToolCalls:         calls,
		ParsedToolCalls:   parsed,
		CitationLinks:     snapshot.CitationLinks,
		ContentFilter:     snapshot.ContentFilter,
		UpstreamError:     snapshot.UpstreamError,
		ResponseMessageID: snapshot.ResponseMessageID,
		StopReason:        stopReason,
	}
	turn.Usage = buildUsageWithPrefill(opts, thinking, text)
	if !snapshot.AlreadyEmittedCalls && !snapshot.AlreadyEmittedToolRaw {
		turn.Error = validateToolOutput(turn, opts, repairCompleted)
	}
	if turn.Error != nil {
		turn.StopReason = StopReasonError
	}
	return turn
}

// buildUsageWithPrefill is BuildUsage with the BuildOptions prompt-count
// prefill channel (see BuildOptions.PromptTokenCountFn).
func buildUsageWithPrefill(opts BuildOptions, thinking, text string) Usage {
	inputTokens := 0
	prefilled := false
	if opts.PromptTokenCountFn != nil {
		if count, ok := opts.PromptTokenCountFn(opts.Prompt); ok {
			inputTokens = count
			prefilled = true
		}
	}
	if !prefilled {
		inputTokens = util.CountPromptTokens(opts.Prompt, opts.Model)
	}
	inputTokens += opts.RefFileTokens
	reasoningTokens := util.CountOutputTokens(thinking, opts.Model)
	outputTokens := reasoningTokens + util.CountOutputTokens(text, opts.Model)
	return Usage{
		InputTokens:     inputTokens,
		OutputTokens:    outputTokens,
		ReasoningTokens: reasoningTokens,
		TotalTokens:     inputTokens + outputTokens,
	}
}

func BuildUsage(model, prompt, thinking, text string, refFileTokens int) Usage {
	inputTokens := util.CountPromptTokens(prompt, model) + refFileTokens
	reasoningTokens := util.CountOutputTokens(thinking, model)
	outputTokens := reasoningTokens + util.CountOutputTokens(text, model)
	return Usage{
		InputTokens:     inputTokens,
		OutputTokens:    outputTokens,
		ReasoningTokens: reasoningTokens,
		TotalTokens:     inputTokens + outputTokens,
	}
}

func ValidateTurn(turn Turn, policy promptcompat.ToolChoicePolicy) *OutputError {
	// Preserve explicit upstream errors even after partial text. A truncated
	// answer must not be reported as successful or regenerated after delivery.
	if strings.TrimSpace(turn.UpstreamError) != "" {
		return &OutputError{Status: http.StatusBadRequest, Message: turn.UpstreamError, Code: "upstream_error"}
	}
	if len(turn.ToolCalls) == 0 && strings.TrimSpace(turn.Text) == "" {
		status, message, code := UpstreamEmptyOutputDetail(turn.ContentFilter, turn.Text, turn.Thinking)
		return &OutputError{Status: status, Message: message, Code: code}
	}
	if policy.DisableParallel && len(turn.ToolCalls) > 1 {
		return &OutputError{Status: http.StatusUnprocessableEntity, Message: "parallel_tool_calls=false permits at most one tool call per response.", Code: "tool_choice_violation"}
	}
	if policy.Constrained() || len(policy.Allowed) > 0 {
		for _, call := range turn.ToolCalls {
			if policy.IsNone() || !policy.Allows(call.Name) || (policy.Mode == promptcompat.ToolChoiceForced && call.Name != policy.ForcedName) {
				return &OutputError{Status: http.StatusUnprocessableEntity, Message: "tool_choice forbids this tool call: " + call.Name, Code: "tool_choice_violation"}
			}
		}
	}
	if policy.IsRequired() && len(turn.ToolCalls) == 0 {
		return &OutputError{
			Status:  http.StatusUnprocessableEntity,
			Message: "tool_choice requires at least one valid tool call.",
			Code:    "tool_choice_violation",
		}
	}
	return nil
}

func UpstreamEmptyOutputDetail(contentFilter bool, text, thinking string) (int, string, string) {
	_ = text
	if contentFilter {
		return http.StatusBadRequest, "Upstream content filtered the response and returned no output.", "content_filter"
	}
	if strings.TrimSpace(thinking) != "" {
		if IsModerationBlockedThinking(thinking) {
			return http.StatusBadRequest, "输入内容被审核拦截", "input_content_blocked"
		}
		return http.StatusTooManyRequests, "Upstream account hit a rate limit and returned reasoning without visible output.", "upstream_empty_output"
	}
	return http.StatusServiceUnavailable, "Upstream service is unavailable and returned no output.", "upstream_unavailable"
}

// ShouldRetryEmptyOutput returns true when the turn produced no visible text
// and has no tool calls or content filter. This includes thinking-only responses,
// where the model returned reasoning but no answer — a retry may yield text.
func ShouldRetryEmptyOutput(turn Turn, attempts, maxAttempts int) bool {
	if turn.Error != nil && (turn.Error.Code == "invalid_tool_arguments" || turn.Error.Code == "tool_choice_violation") {
		return false
	}
	return attempts < maxAttempts &&
		!turn.ContentFilter &&
		len(turn.ToolCalls) == 0 &&
		strings.TrimSpace(turn.UpstreamError) == "" &&
		strings.TrimSpace(turn.Text) == ""
}

func FinalizeTurn(turn Turn, opts FinalizeOptions) FinalOutcome {
	hasToolCalls := len(turn.ToolCalls) > 0 || opts.AlreadyEmittedToolCalls
	hasVisibleText := strings.TrimSpace(turn.Text) != ""
	hasVisibleThinking := strings.TrimSpace(turn.Thinking) != ""
	err := turn.Error
	if hasToolCalls && (err == nil || err.Code != "tool_choice_violation" && err.Code != "invalid_tool_arguments" && err.Code != "upstream_read_error") {
		err = nil
	}
	finishReason := FinishReason(turn)
	if hasToolCalls {
		finishReason = "tool_calls"
	}
	return FinalOutcome{
		FinishReason:     finishReason,
		Error:            err,
		Usage:            turn.Usage,
		HasToolCalls:     hasToolCalls,
		HasVisibleText:   hasVisibleText,
		HasVisibleOutput: hasVisibleText || hasVisibleThinking || hasToolCalls,
		ShouldFail:       err != nil,
	}
}

func OpenAIChatUsage(turn Turn) map[string]any {
	return map[string]any{
		"prompt_tokens":     turn.Usage.InputTokens,
		"completion_tokens": turn.Usage.OutputTokens,
		"total_tokens":      turn.Usage.TotalTokens,
		"completion_tokens_details": map[string]any{
			"reasoning_tokens": turn.Usage.ReasoningTokens,
		},
	}
}

func OpenAIResponsesUsage(turn Turn) map[string]any {
	return map[string]any{
		"input_tokens":  turn.Usage.InputTokens,
		"output_tokens": turn.Usage.OutputTokens,
		"total_tokens":  turn.Usage.TotalTokens,
	}
}

func FinishReason(turn Turn) string {
	switch turn.StopReason {
	case StopReasonToolCalls:
		return "tool_calls"
	case StopReasonContentFilter:
		return "content_filter"
	default:
		return "stop"
	}
}
