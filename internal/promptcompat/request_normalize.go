package promptcompat

import (
	"fmt"
	"strings"

	"ds2api/internal/config"
	"ds2api/internal/toolcall"
	"ds2api/internal/util"
)

// firstToolMarker returns the optional caller-specific tool-call marker. It is
// variadic so existing callers (and tests) that predate the marker keep working.
func firstToolMarker(markers []string) string {
	if len(markers) == 0 {
		return ""
	}
	return strings.TrimSpace(markers[0])
}

// applyGeneratedToolMarker rewrites the canonical EPSE keyword inside GENERATED
// prompt blocks (format spec, tool instructions, bottom reminder) with the
// caller-specific marker. This mirrors the upstream ds2api whole-string
// rewrite: the aligned wording names the keyword in prose, so the
// delimiter-only toolcall.ApplyToolMarker would leak the canonical keyword for
// per-key callers. History and user content keep using toolcall.ApplyToolMarker.
func applyGeneratedToolMarker(text, marker string) string {
	marker = strings.TrimSpace(marker)
	if marker == "" || marker == toolcall.EPSEKeyword || text == "" {
		return text
	}
	return strings.ReplaceAll(text, toolcall.EPSEKeyword, marker)
}

func NormalizeOpenAIChatRequest(store config.ModelAliasReader, req map[string]any, traceID string, toolMarker ...string) (StandardRequest, error) {
	marker := firstToolMarker(toolMarker)
	model, _ := req["model"].(string)
	messagesRaw, _ := req["messages"].([]any)
	if strings.TrimSpace(model) == "" || len(messagesRaw) == 0 {
		return StandardRequest{}, fmt.Errorf("request must include 'model' and 'messages'")
	}
	if err := ValidateToolHistory(messagesRaw); err != nil {
		return StandardRequest{}, err
	}
	resolvedModel, ok := config.ResolveModel(store, model)
	if !ok {
		return StandardRequest{}, fmt.Errorf("model %q is not available", model)
	}
	defaultThinkingEnabled, searchEnabled, _ := config.GetModelConfig(resolvedModel)
	thinkingEnabled := util.ResolveThinkingEnabled(req, defaultThinkingEnabled)
	if config.IsNoThinkingModel(resolvedModel) {
		thinkingEnabled = false
	}
	responseModel := strings.TrimSpace(model)
	if responseModel == "" {
		responseModel = resolvedModel
	}
	toolsDisabled := ToolsForceDisabledInRequest(req)
	var toolPolicy ToolChoicePolicy
	if toolsDisabled {
		toolPolicy = ToolChoicePolicy{Mode: ToolChoiceNone}
	} else {
		var err error
		toolPolicy, err = parseOpenAIToolPolicy(req)
		if err != nil {
			return StandardRequest{}, err
		}
	}
	reminder := ResolveToolReminder(store)
	toolPolicy = ApplyToolSafetySettings(store, toolPolicy)
	finalPrompt, toolNames := BuildOpenAIPrompt(messagesRaw, req["tools"], traceID, toolPolicy, thinkingEnabled, reminder, marker)
	toolNames = ensureToolDetectionEnabled(toolNames, req["tools"])
	passThrough := collectOpenAIChatPassThrough(req)
	refFileIDs := CollectOpenAIRefFileIDs(req)

	return StandardRequest{
		Surface:              "openai_chat",
		RequestedModel:       strings.TrimSpace(model),
		ResolvedModel:        resolvedModel,
		ResponseModel:        responseModel,
		Messages:             messagesRaw,
		PromptTokenText:      finalPrompt,
		ToolsRaw:             req["tools"],
		FinalPrompt:          finalPrompt,
		ToolNames:            toolNames,
		ToolChoice:           toolPolicy,
		ToolsDisabled:        toolsDisabled,
		Stream:               util.ToBool(req["stream"]),
		Thinking:             thinkingEnabled,
		Search:               searchEnabled,
		RefFileIDs:           refFileIDs,
		RefFileTokens:        EstimateInlineFileTokens(req),
		PassThrough:          passThrough,
		ToolMarker:           marker,
		ToolReminderDisabled: !reminder.Enabled,
		ToolReminderPrompt:   reminder.Prompt,
	}, nil
}

func NormalizeOpenAIResponsesRequest(store config.ModelAliasReader, req map[string]any, traceID string, toolMarker ...string) (StandardRequest, error) {
	marker := firstToolMarker(toolMarker)
	model, _ := req["model"].(string)
	model = strings.TrimSpace(model)
	if model == "" {
		return StandardRequest{}, fmt.Errorf("request must include 'model'")
	}
	resolvedModel, ok := config.ResolveModel(store, model)
	if !ok {
		return StandardRequest{}, fmt.Errorf("model %q is not available", model)
	}
	defaultThinkingEnabled, searchEnabled, _ := config.GetModelConfig(resolvedModel)
	thinkingEnabled := util.ResolveThinkingEnabled(req, defaultThinkingEnabled)
	if config.IsNoThinkingModel(resolvedModel) {
		thinkingEnabled = false
	}

	messagesRaw := ResponsesMessagesFromRequest(req)
	if err := ValidateToolHistory(messagesRaw); err != nil {
		return StandardRequest{}, err
	}
	if len(messagesRaw) == 0 {
		return StandardRequest{}, fmt.Errorf("request must include 'input' or 'messages'")
	}
	toolsDisabled := ToolsForceDisabledInRequest(req)
	reminder := ResolveToolReminder(store)
	var toolPolicy ToolChoicePolicy
	if toolsDisabled {
		toolPolicy = ToolChoicePolicy{Mode: ToolChoiceNone}
	} else {
		parsed, err := parseOpenAIToolPolicy(req)
		if err != nil {
			return StandardRequest{}, err
		}
		toolPolicy = parsed
	}
	toolPolicy = ApplyToolSafetySettings(store, toolPolicy)
	finalPrompt, toolNames := BuildOpenAIPrompt(messagesRaw, req["tools"], traceID, toolPolicy, thinkingEnabled, reminder, marker)
	toolNames = ensureToolDetectionEnabled(toolNames, req["tools"])
	if !toolPolicy.IsNone() {
		if len(toolPolicy.Allowed) == 0 {
			toolPolicy.Allowed = namesToSet(toolNames)
		}
	}
	passThrough := collectOpenAIChatPassThrough(req)
	refFileIDs := CollectOpenAIRefFileIDs(req)

	return StandardRequest{
		Surface:              "openai_responses",
		RequestedModel:       model,
		ResolvedModel:        resolvedModel,
		ResponseModel:        model,
		Messages:             messagesRaw,
		PromptTokenText:      finalPrompt,
		ToolsRaw:             req["tools"],
		FinalPrompt:          finalPrompt,
		ToolNames:            toolNames,
		ToolChoice:           toolPolicy,
		ToolsDisabled:        toolsDisabled,
		Stream:               util.ToBool(req["stream"]),
		Thinking:             thinkingEnabled,
		Search:               searchEnabled,
		RefFileIDs:           refFileIDs,
		RefFileTokens:        EstimateInlineFileTokens(req),
		PassThrough:          passThrough,
		ToolMarker:           marker,
		ToolReminderDisabled: !reminder.Enabled,
		ToolReminderPrompt:   reminder.Prompt,
	}, nil
}

func ensureToolDetectionEnabled(toolNames []string, toolsRaw any) []string {
	if len(toolNames) > 0 {
		return toolNames
	}
	tools, _ := toolsRaw.([]any)
	if len(tools) == 0 {
		return toolNames
	}
	// Keep stream sieve/tool buffering enabled even when client tool schemas
	// are malformed or lack explicit names; parsed tool payload names are no
	// longer filtered by this list.
	return []string{"__any_tool__"}
}

func collectOpenAIChatPassThrough(req map[string]any) map[string]any {
	out := map[string]any{}
	for _, k := range []string{
		"temperature",
		"top_p",
		"max_tokens",
		"max_completion_tokens",
		"presence_penalty",
		"frequency_penalty",
		"stop",
	} {
		if v, ok := req[k]; ok {
			out[k] = v
		}
	}
	return out
}

// EstimateInlineFileTokens extracts the byte count stashed by PreprocessInlineFileInputs
// and converts it to a conservative token estimate. Inline files are typically images or
// documents that the upstream model will process; we use bytes/3 (rather than bytes/4)
// as a slightly pessimistic approximation so the returned context token count stays
// safely above the real value.
func EstimateInlineFileTokens(req map[string]any) int {
	raw, ok := req["_inline_file_bytes"]
	if !ok {
		return 0
	}
	var bytes int
	switch v := raw.(type) {
	case int:
		bytes = v
	case int64:
		bytes = int(v)
	case float64:
		bytes = int(v)
	default:
		return 0
	}
	if bytes <= 0 {
		return 0
	}
	return bytes / 3
}
