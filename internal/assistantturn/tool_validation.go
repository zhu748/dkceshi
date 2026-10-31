package assistantturn

import (
	"net/http"
	"strings"

	"ds2api/internal/sse"
	"ds2api/internal/toolcall"
)

// Validate before rendering on every protocol. A known client invocation that
// still has broken syntax is not an answer and must not execute partially.
func validateToolOutput(turn Turn, opts BuildOptions, repaired bool) *OutputError {
	if turn.ContentCut == sse.CutPromptEcho {
		return &OutputError{Status: http.StatusBadGateway, Code: OutputCodePromptEcho,
			Message: "Upstream repeated generated instructions instead of producing an answer."}
	}
	parsed := turn.ParsedToolCalls
	if !repaired && (parsed.SawToolCallIntent || parsed.SawToolCallSyntax && len(turn.ToolCalls) == 0) &&
		!turn.ContentFilter && strings.TrimSpace(turn.UpstreamError) == "" {
		unresolved := parsed.ResidualIntentText
		if strings.TrimSpace(unresolved) == "" {
			unresolved = parsed.SourceText
		}
		if strings.TrimSpace(unresolved) == "" {
			unresolved = turn.RawText
		}
		// A damaged undeclared call must not disappear beside a valid call.
		// Empty names also covers schema-less continuation requests.
		if toolcall.HasNamedToolInvocation(unresolved, nil) {
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
