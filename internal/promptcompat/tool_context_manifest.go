package promptcompat

import (
	"strconv"
	"strings"

	"ds2api/internal/prompt"
	"ds2api/internal/toolcall"
)

// Paths refer to the normalized catalog. InputFields retains adapter source
// paths, including Gemini envelopes. Destinations identify serializers, not
// evidence that the upstream model read or understood each value.
type ContextToolFieldDigest struct {
	ContextFieldDigest
	ToolIndex    int      `json:"tool_index"`
	ToolName     string   `json:"tool_name,omitempty"`
	Destinations []string `json:"destinations,omitempty"`
	OmittedPaths []string `json:"omitted_paths,omitempty"`
}

func (r StandardRequest) toolContextFieldDigests() []ContextToolFieldDigest {
	tools, _ := r.ToolsRaw.([]any)
	fields := appendInputItemDigests(nil, tools, "/tools")
	out := make([]ContextToolFieldDigest, 0, len(fields))
	for _, field := range fields {
		entry := ContextToolFieldDigest{ContextFieldDigest: field, ToolIndex: -1}
		if field.Disposition == "adapter_source_trace_limit" {
			out = append(out, entry)
			continue
		}
		parts := strings.Split(field.Path, "/")
		if len(parts) < 4 || len(parts) > 5 || len(parts) == 5 && parts[3] != "function" {
			continue // An extension's nested business JSON is represented by its parent.
		}
		index, err := strconv.Atoi(parts[2])
		if err != nil || index < 0 || index >= len(tools) {
			continue
		}
		tool, _ := tools[index].(map[string]any)
		entry.ToolIndex = index
		entry.ToolName, _, _ = toolcall.ExtractToolMeta(tool)
		entry.Disposition = toolDefinitionFieldKind(tool, field.Name, len(parts) > 4)
		object := tool
		if len(parts) > 4 {
			object, _ = tool["function"].(map[string]any)
		}
		switch {
		case entry.Disposition == "transport_only":
			entry.OmittedReason = "transport_hint"
		case r.ToolsDisabled || r.ToolChoice.IsNone():
			entry.OmittedReason = "tools_disabled_by_policy"
		case !r.ToolChoice.Allows(entry.ToolName):
			entry.OmittedReason = "tool_excluded_by_policy"
		default:
			entry.Destinations = []string{"live_prompt"}
			if r.CurrentToolsFileID != "" {
				entry.Destinations = []string{"tools_file"}
			}
			if entry.Disposition == "tool_context" {
				for _, path := range prompt.ContextValueOmittedPaths(object[field.Name]) {
					entry.OmittedPaths = append(entry.OmittedPaths, field.Path+path)
				}
			}
		}
		out = append(out, entry)
	}
	return out
}
