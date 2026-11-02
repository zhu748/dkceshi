package assistantturn

import (
	"ds2api/internal/sse"
	"ds2api/internal/tooldiagnostics"
)

// Keep unfinished streams without repair or replacing existing validated evidence.
func ObserveFailedOutput(text, thinking, detection string, opts BuildOptions, failure *OutputError) {
	ctx := opts.DiagnosticContext
	if tooldiagnostics.FromContext(ctx) == nil || failure == nil {
		return
	}
	if tooldiagnostics.HasObservation(ctx) {
		tooldiagnostics.Failure(ctx, failure.Code, failure.Message)
		return
	}
	parseOptions := opts
	parseOptions.DiagnosticContext, parseOptions.ToolCallRepairCtx, parseOptions.ToolCallRepair = nil, nil, nil
	turn := BuildTurnFromCollected(sse.CollectResult{Text: text, Thinking: thinking, ToolDetectionThinking: detection, UpstreamError: failure.Message}, parseOptions)
	turn.Error = failure
	if detection == "" {
		detection = thinking
	}
	observeToolDiagnostics(turn, opts, text, detection)
}
