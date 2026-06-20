package promptcompat

import (
        "ds2api/internal/prompt"
)

func buildOpenAIFinalPrompt(messagesRaw []any, toolsRaw any, traceID string, thinkingEnabled bool) (string, []string) {
        return BuildOpenAIPrompt(messagesRaw, toolsRaw, traceID, DefaultToolChoicePolicy(), thinkingEnabled)
}

func BuildOpenAIPrompt(messagesRaw []any, toolsRaw any, traceID string, toolPolicy ToolChoicePolicy, thinkingEnabled bool) (string, []string) {
        return buildOpenAIPrompt(messagesRaw, toolsRaw, traceID, toolPolicy, thinkingEnabled, true)
}

func BuildOpenAIPromptWithToolInstructionsOnly(messagesRaw []any, toolsRaw any, traceID string, toolPolicy ToolChoicePolicy, thinkingEnabled bool) (string, []string) {
        return buildOpenAIPrompt(messagesRaw, toolsRaw, traceID, toolPolicy, thinkingEnabled, false)
}

func buildOpenAIPrompt(messagesRaw []any, toolsRaw any, traceID string, toolPolicy ToolChoicePolicy, thinkingEnabled bool, includeToolDescriptions bool) (string, []string) {
        messages := NormalizeOpenAIMessagesForPrompt(messagesRaw, traceID)
        toolNames := []string{}
        if tools, ok := toolsRaw.([]any); ok && len(tools) > 0 {
                if includeToolDescriptions {
                        messages, toolNames = injectToolPrompt(messages, tools, toolPolicy)
                } else {
                        messages, toolNames = injectToolPromptInstructionsOnly(messages, tools, toolPolicy)
                }
        }
        // hasTools 以「最终是否真的注入了工具指令」为准：toolNames 非空表示本次请求会
        // 触发 DSML 工具调用解析；为空时（tools 缺失或 ToolChoicePolicy.IsNone()）
        // 仍可能因历史含 tool 消息而需要 Output integrity guard，由 prompt 包内自行判断。
        hasTools := len(toolNames) > 0
        return prompt.MessagesPrepareWithThinkingAndToolHint(messages, thinkingEnabled, hasTools), toolNames
}

// BuildOpenAIPromptForAdapter exposes the OpenAI-compatible prompt building flow so
// other protocol adapters (for example Gemini) can reuse the same tool/history
// normalization logic and remain behavior-compatible with chat/completions.
func BuildOpenAIPromptForAdapter(messagesRaw []any, toolsRaw any, traceID string, thinkingEnabled bool) (string, []string) {
        return buildOpenAIFinalPrompt(messagesRaw, toolsRaw, traceID, thinkingEnabled)
}
