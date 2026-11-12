package usagestats

import "ds2api/internal/tokenusage"

// ParseUsage 从 OpenAI/Claude/Gemini 风格的 usage map 中提取
// prompt/completion/reasoning/total tokens。
func ParseUsage(usage map[string]any) (prompt, completion, reasoning, total int64) {
	if usage == nil {
		return 0, 0, 0, 0
	}
	prompt = firstInt(usage, "prompt_tokens", "input_tokens")
	completion = firstInt(usage, "completion_tokens", "output_tokens")
	reasoning = firstInt(usage, "reasoning_tokens")

	if details, ok := usage["completion_tokens_details"].(map[string]any); ok {
		reasoning = firstInt(details, "reasoning_tokens")
		if reasoning == 0 {
			reasoning = firstInt(usage, "reasoning_tokens")
		}
	}

	total = firstInt(usage, "total_tokens")
	if total == 0 {
		total = prompt + completion
	}
	if total < prompt+completion {
		total = prompt + completion
	}
	return prompt, completion, reasoning, total
}

func firstInt(source map[string]any, keys ...string) int64 {
	for _, key := range keys {
		if n, ok := tokenusage.Count(source[key]); ok {
			return int64(n)
		}
	}
	return 0
}
