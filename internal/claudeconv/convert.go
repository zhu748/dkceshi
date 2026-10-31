package claudeconv

import (
	"strings"

	"ds2api/internal/config"
)

// claudeSystemText 提取 Anthropic 顶层 `system` 字段的文本。
// 协议允许两种形态：纯字符串，或 content block 数组
// （[{"type":"text","text":"..."}]，block 上可带 cache_control 等附加字段）。
// 数组形态下拼接所有 text block；无法识别的形态返回空串（与旧行为一致，
// 不再静默丢掉整个 system prompt）。
func claudeSystemText(system any) string {
	switch v := system.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, item := range v {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if t, _ := block["type"].(string); t != "" && t != "text" {
				continue
			}
			if text, _ := block["text"].(string); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
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
	if system := claudeSystemText(claudeReq["system"]); system != "" {
		convertedMessages = append(convertedMessages, map[string]any{"role": "system", "content": system})
	}
	convertedMessages = append(convertedMessages, messages...)

	out := map[string]any{"model": dsModel, "messages": convertedMessages}
	for _, k := range []string{"temperature", "top_p", "stream"} {
		if v, ok := claudeReq[k]; ok {
			out[k] = v
		}
	}
	if stopSeq, ok := claudeReq["stop_sequences"]; ok {
		out["stop"] = stopSeq
	}
	return out
}
