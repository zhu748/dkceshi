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
	inputContext := InputFieldDigests(req)
	var err error
	req, err = NormalizeLegacyToolDefinitions(req)
	if err != nil {
		return StandardRequest{}, err
	}
	marker := firstToolMarker(toolMarker)
	model, _ := req["model"].(string)
	messagesRaw, _ := req["messages"].([]any)
	if strings.TrimSpace(model) == "" || len(messagesRaw) == 0 {
		return StandardRequest{}, fmt.Errorf("request must include 'model' and 'messages'")
	}
	messagesRaw, err = normalizeValidatedToolHistory(messagesRaw)
	if err != nil {
		return StandardRequest{}, err
	}
	resolvedModel, ok := config.ResolveModel(store, model)
	if !ok {
		return StandardRequest{}, fmt.Errorf("model %q is not available", model)
	}
	defaultThinkingEnabled, searchEnabled, _ := config.GetModelConfig(resolvedModel)
	// 思考开关只由模型名决定：-nothinking 后缀关闭，无后缀开启（GetModelConfig
	// 已按后缀返回开关）。请求里的 thinking / reasoning / reasoning_effort
	// 仅兼容接收，不再折叠成布尔——App 的真实开关就是所选模型。
	thinkingEnabled := defaultThinkingEnabled
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
	refFileIDs := CollectOpenAIRefFileIDs(req)

	standard := StandardRequest{
		Surface:              "openai_chat",
		InputContext:         inputContext,
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
		ToolMarker:           marker,
		ToolReminderDisabled: !reminder.Enabled,
		ToolReminderPrompt:   reminder.Prompt,
	}
	return applyOpenAIOutputFormat(standard, req)
}

func NormalizeOpenAIResponsesRequest(store config.ModelAliasReader, req map[string]any, traceID string, toolMarker ...string) (StandardRequest, error) {
	if err := ValidateResponsesRequestContext(req); err != nil {
		return StandardRequest{}, err
	}
	inputContext := InputFieldDigests(req)
	var err error
	req, err = NormalizeLegacyToolDefinitions(req)
	if err != nil {
		return StandardRequest{}, err
	}
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
	// 同上：思考开关只由模型名决定，请求参数不参与。
	thinkingEnabled := defaultThinkingEnabled

	messagesRaw := ResponsesMessagesFromRequest(req)
	messagesRaw, err = normalizeValidatedToolHistory(messagesRaw)
	if err != nil {
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
	refFileIDs := CollectOpenAIRefFileIDs(req)

	standard := StandardRequest{
		Surface:              "openai_responses",
		InputContext:         inputContext,
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
		ToolMarker:           marker,
		ToolReminderDisabled: !reminder.Enabled,
		ToolReminderPrompt:   reminder.Prompt,
	}
	return applyOpenAIOutputFormat(standard, req)
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

// EstimateInlineFileTokens prefers per-file upstream counts resolved by shared
// preprocessing. The byte estimate remains for older internal callers only.
func EstimateInlineFileTokens(req map[string]any) int {
	if tokens, ok := referenceFileTokens(req); ok {
		return tokens
	}
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
