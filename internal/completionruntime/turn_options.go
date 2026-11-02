package completionruntime

import (
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
