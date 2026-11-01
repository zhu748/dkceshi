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
	if err := terminalOutputError(turn); err != nil {
		return err
	}
	if turn.ContentCut == sse.CutPromptEcho {
		return &OutputError{Status: http.StatusBadGateway, Code: OutputCodePromptEcho,
			Message: "Upstream repeated generated instructions instead of producing an answer."}
	}
	if turn.ContentCut == sse.CutOutputLimit {
		return &OutputError{Status: 502, Code: "output_limit_exceeded", Message: "Upstream output exceeded configured resource limits."}
	}
	parsed := turn.ParsedToolCalls
	if !repaired {
		unresolved := parsed.ResidualProtocolText
		if unresolved == "" {
			unresolved = parsed.ResidualIntentText
		}
		if strings.TrimSpace(unresolved) == "" && len(turn.ToolCalls) == 0 {
			unresolved = parsed.SourceText
		}
		if strings.TrimSpace(unresolved) == "" && len(turn.ToolCalls) == 0 {
			unresolved = turn.RawText
		}
		// A damaged undeclared call must not disappear beside a valid call.
		// Empty names also covers schema-less continuation requests.
		if toolcall.HasNamedToolInvocation(unresolved, nil) || toolcall.HasUnresolvedToolMarkup(unresolved) {
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

// Terminal upstream states win over parsed calls, repair and retry decisions.
func terminalOutputError(turn Turn) *OutputError {
	if strings.TrimSpace(turn.UpstreamError) != "" {
		return &OutputError{Status: http.StatusBadRequest, Message: turn.UpstreamError, Code: "upstream_error"}
	}
	if turn.ContentFilter {
		return &OutputError{Status: http.StatusBadRequest, Message: "Upstream content filtered the response.", Code: "content_filter"}
	}
	return nil
}
