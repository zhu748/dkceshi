package shared

import (
	"ds2api/internal/config"
	"ds2api/internal/promptcompat"
)

// ApplyThinkingInjection 在 prompt 组装前把思考格式提示词追加到最新一条 user 消息末尾。
//
// 触发条件（三道门全部满足）：
//  1. store 非空（链路上有可读的配置 / 默认 prompt）
//  2. stdReq.Thinking=true（请求本身要求思考；-nothinking 后缀模型这里会是 false）
//  3. 全局 thinking_injection.enabled=true，或者本次请求的模型带 -thinkinginject 后缀
//     （后缀用于按需启用：全局关闭时仍然注入，全局开启时跟随全局）
func ApplyThinkingInjection(store ConfigReader, stdReq promptcompat.StandardRequest) promptcompat.StandardRequest {
	if store == nil || !stdReq.Thinking {
		return stdReq
	}
	injectEnabled := store.ThinkingInjectionEnabled()
	if !injectEnabled && !config.IsThinkingInjectModel(stdReq.ResolvedModel) {
		return stdReq
	}
	messages, changed := promptcompat.AppendThinkingInjectionPromptToLatestUser(stdReq.Messages, store.ThinkingInjectionPrompt())
	if !changed {
		return stdReq
	}
	finalPrompt, toolNames := promptcompat.BuildOpenAIPrompt(messages, stdReq.ToolsRaw, "", stdReq.ToolChoice, stdReq.Thinking)
	if len(toolNames) == 0 && len(stdReq.ToolNames) > 0 {
		toolNames = stdReq.ToolNames
	}
	stdReq.Messages = messages
	stdReq.FinalPrompt = finalPrompt
	stdReq.ToolNames = toolNames
	return stdReq
}
