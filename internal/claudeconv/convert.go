package claudeconv

import (
	"strings"

	"ds2api/internal/config"
	"ds2api/internal/prompt"
)

// Retain structured system blocks until shared upload and rendering. Flattening
// here would scrub attachment bytes before an internal Chat bridge uploads them.
func claudeSystemContent(system any) any {
	switch v := system.(type) {
	case string:
		return v
	case []any:
		var parts []any
		structured := false
		for _, item := range v {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if t, _ := block["type"].(string); t != "" && t != "text" {
				continue
			}
			if _, ok := block["text"].(string); ok {
				structured = structured || block["attachments"] != nil || block["files"] != nil
				copy := make(map[string]any, len(block)+1)
				for key, value := range block {
					copy[key] = value
				}
				copy["type"] = "text"
				parts = append(parts, copy)
			}
		}
		if !structured {
			return prompt.NormalizeContent(parts)
		}
		return parts
	default:
		return ""
	}
}

func ConvertClaudeToDeepSeek(claudeReq map[string]any, aliasProvider config.ModelAliasReader, defaultClaudeModel string) map[string]any {
	messages, _ := claudeReq["messages"].([]any)
	model, _ := claudeReq["model"].(string)
	if model == "" {
		model = defaultClaudeModel
	}

	dsModel, ok := config.ResolveModel(aliasProvider, model)
	if !ok || strings.TrimSpace(dsModel) == "" {
		dsModel = "deepseek-v4.1-flash"
	}

	convertedMessages := make([]any, 0, len(messages)+1)
	if system := claudeSystemContent(claudeReq["system"]); prompt.NormalizeContent(system) != "" {
		convertedMessages = append(convertedMessages, map[string]any{"role": "system", "content": system})
	}
	convertedMessages = append(convertedMessages, messages...)

	// 只保留对话内容与模型解析结果。采样参数（temperature/top_p）、停止序列与
	// 思考强度在 App 反代上没有对应控制，一律不进入内部请求形态，避免下游
	// 误把它们透传到上游请求体。
	return map[string]any{"model": dsModel, "messages": convertedMessages}
}
