package assistantturn

import (
	"encoding/json"
	"strings"

	"ds2api/internal/toolcall"
	"ds2api/internal/tooldiagnostics"
)

func observeToolDiagnostics(turn Turn, opts BuildOptions, originalText, originalThinking string) {
	ctx := opts.DiagnosticContext
	if ctx == nil {
		ctx = opts.ToolCallRepairCtx
	}
	if tooldiagnostics.FromContext(ctx) == nil {
		return
	}
	p := turn.ParsedToolCalls
	o := tooldiagnostics.Observation{Relevant: len(turn.ToolCalls) > 0 || p.SawToolCallSyntax || p.SawToolCallIntent || opts.ToolChoice.IsRequired(), Model: opts.Model, Source: "content", Raw: originalText, Residual: p.ResidualProtocolText, LocalSteps: append([]string{}, p.RecoverySteps...)}
	if p.SourceChannel == "reasoning" {
		o.Source, o.Raw = "reasoning", originalThinking
	}
	if !p.SawToolCallIntent && !toolcall.HasUnresolvedToolMarkup(p.ResidualProtocolText) {
		o.Residual = ""
	}
	if turn.Error != nil {
		o.ErrorCode, o.Error = turn.Error.Code, turn.Error.Message
	}
	// The batch is atomic. Individual syntax/schema successes do not imply
	// that any call from a rejected batch was delivered.
	for i, call := range turn.ToolCalls {
		item := tooldiagnostics.Call{Ordinal: i + 1, Name: call.Name, Parse: "success", Schema: "not_provided", Delivery: "not_delivered"}
		raw, err := json.Marshal(call.Input)
		if err == nil {
			item.Arguments = tooldiagnostics.Capture(string(raw))
		}
		o.LocalSteps = append(o.LocalSteps, call.RecoverySteps...)
		if turn.Error != nil && turn.Error.Code != "invalid_tool_arguments" && turn.Error.Code != "tool_choice_violation" {
			item.Schema = "skipped"
		} else if err := toolcall.ValidateToolArguments([]toolcall.ParsedToolCall{call}, opts.ToolsRaw); err != nil {
			item.Schema, item.Error = "failed", err.Error()
		} else if toolHasSchema(opts.ToolsRaw, call.Name) {
			item.Schema = "passed"
		}
		o.Calls = append(o.Calls, item)
	}
	// The failed residual is recorded separately so a parsed sibling is never
	// mistaken for the damaged invocation, even when both have the same name.
	if turn.Error != nil && strings.TrimSpace(o.Residual) != "" {
		for _, name := range toolcall.DiagnosticToolNames(p.ResidualProtocolText) {
			o.Calls = append(o.Calls, tooldiagnostics.Call{Ordinal: 0, Name: name, Parse: "failed", Schema: "skipped", Error: turn.Error.Message, Delivery: "not_delivered"})
		}
	}
	if o.Relevant && len(o.Calls) == 0 && turn.Error != nil {
		o.Calls = []tooldiagnostics.Call{{Ordinal: 1, Name: "", Parse: "failed", Schema: "skipped", Error: turn.Error.Message, Delivery: "not_delivered"}}
	}
	tooldiagnostics.Observe(ctx, o)
}

func toolHasSchema(raw any, name string) bool {
	tools, _ := raw.([]any)
	for _, value := range tools {
		tool, _ := value.(map[string]any)
		n, _, schema := toolcall.ExtractToolMeta(tool)
		if n == name {
			return schema != nil
		}
	}
	return false
}
