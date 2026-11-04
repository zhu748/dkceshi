package completionruntime

import (
	"context"
	"ds2api/internal/assistantturn"
	"ds2api/internal/promptcompat"
)

func buildOptions(stdReq promptcompat.StandardRequest, prompt string, opts Options) assistantturn.BuildOptions {
	built := buildOptionsWithoutRepair(stdReq, prompt, opts)
	built.DiagnosticContext = opts.diagnosticCtx
	built.ToolCallRepair = opts.toolCallRepair
	built.ToolCallRepairCtx = opts.toolCallRepairCtx
	return built
}

// buildOptionsWithoutRepair mirrors buildOptions but never attaches the LLM
// repair invoker. Intermediate builds (per-attempt collection) must use this so
// a single upstream output triggers at most one LLM repair pass.
func buildOptionsWithoutRepair(stdReq promptcompat.StandardRequest, prompt string, opts Options) assistantturn.BuildOptions {
	return assistantturn.BuildOptions{
		Model:                 stdReq.ResponseModel,
		OutputFormat:          stdReq.OutputFormat,
		Prompt:                prompt,
		RefFileTokens:         stdReq.RefFileTokens,
		SearchEnabled:         stdReq.Search,
		StripReferenceMarkers: opts.StripReferenceMarkers,
		ToolNames:             stdReq.ToolNames,
		ToolsRaw:              stdReq.ToolsRaw,
		ToolChoice:            stdReq.ToolChoice,
		ToolMarker:            stdReq.ToolMarker,
	}
}

// Interrupted collection exits before the terminal build; retain it once,
// without executing repair on truncated output.
func collectedBuildOptions(ctx context.Context, req promptcompat.StandardRequest, prompt string, opts Options, readError error) assistantturn.BuildOptions {
	built := buildOptionsWithoutRepair(req, prompt, opts)
	if readError != nil {
		built.DiagnosticContext = ctx
	}
	return built
}
