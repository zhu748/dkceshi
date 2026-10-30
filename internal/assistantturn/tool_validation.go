package assistantturn

import (
	"net/http"
	"strings"

	"ds2api/internal/toolcall"
)

// Validate before rendering on every protocol. A known client invocation that
// still has broken syntax is not an answer and must not execute partially.
func validateToolOutput(turn Turn, opts BuildOptions, repaired bool) *OutputError {
	parsed := turn.ParsedToolCalls
	if !repaired && (parsed.SawToolCallIntent || parsed.SawToolCallSyntax && len(turn.ToolCalls) == 0) &&
		!turn.ContentFilter && strings.TrimSpace(turn.UpstreamError) == "" {
		names := opts.ToolNames
		if len(names) == 0 {
			tools, _ := opts.ToolsRaw.([]any)
			for _, raw := range tools {
				tool, _ := raw.(map[string]any)
				if name, _, _ := toolcall.ExtractToolMeta(tool); name != "" {
					names = append(names, name)
				}
			}
		}
		unresolved := parsed.ResidualIntentText
		if strings.TrimSpace(unresolved) == "" {
			unresolved = turn.RawText
		}
		if toolcall.HasNamedToolInvocation(unresolved, names) {
			return &OutputError{
				Status: http.StatusUnprocessableEntity, Code: "invalid_tool_arguments",
				Message: "Upstream returned an incomplete tool call that could not be safely repaired.",
			}
		}
	}
	if err := ValidateTurn(turn, opts.ToolChoice); err != nil {
		return err
	}
	if err := toolcall.ValidateToolArguments(turn.ToolCalls, opts.ToolsRaw); err != nil {
		return &OutputError{Status: http.StatusUnprocessableEntity, Message: err.Error(), Code: "invalid_tool_arguments"}
	}
	return nil
}
