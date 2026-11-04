package promptcompat

import "sort"

// OpenAIChatRequest serializes normalized input for an internal Chat transport.
// Generated prompts never become client message content on this boundary.
func (r StandardRequest) OpenAIChatRequest(original map[string]any) map[string]any {
	out := map[string]any{"model": r.ResolvedModel, "messages": r.Messages, "stream": r.Stream,
		"parallel_tool_calls": !r.ToolChoice.DisableParallel}
	for _, key := range []string{"ref_file_ids", "file_ids", "attachments", "files"} {
		if value, exists := original[key]; exists {
			out[key] = value
		}
	}
	if r.ToolsRaw != nil {
		out["tools"] = r.ToolsRaw
	} else if r.SystemTextToolsDisabled {
		// An explicit empty catalog prevents Chat's text-catalog fallback.
		out["tools"] = []any{}
	}
	policy := r.ToolChoice
	choice := map[string]any{"type": string(policy.Mode)}
	if policy.IsNone() {
		choice["type"] = "none"
	} else if policy.Mode == ToolChoiceForced {
		choice["type"], choice["name"] = "function", policy.ForcedName
	}
	if len(policy.Allowed) > 0 && !policy.IsNone() {
		names := make([]string, 0, len(policy.Allowed))
		for name := range policy.Allowed {
			names = append(names, name)
		}
		sort.Strings(names)
		allowed := make([]any, len(names))
		for i, name := range names {
			allowed[i] = name
		}
		choice["allowed_tools"] = allowed
	}
	out["tool_choice"] = choice
	if r.OutputFormat.Enabled() {
		out["response_format"] = map[string]any{"type": r.OutputFormat.Type, "schema": r.OutputFormat.Schema}
	}
	if r.ToolsDisabled {
		DisableToolsForRequest(out)
	}
	return out
}
