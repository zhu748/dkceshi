package usagestats

import (
	"encoding/json"
	"fmt"
	"strings"
)

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
		value, ok := source[key]
		if !ok || value == nil {
			continue
		}
		if n := toInt64(value); n != 0 {
			return n
		}
	}
	return 0
}

func toInt64(value any) int64 {
	switch v := value.(type) {
	case float64:
		return int64(v)
	case float32:
		return int64(v)
	case int:
		return int64(v)
	case int32:
		return int64(v)
	case int64:
		return v
	case json.Number:
		n, _ := v.Int64()
		return n
	case string:
		var n int64
		if _, err := fmt.Sscanf(strings.TrimSpace(v), "%d", &n); err == nil {
			return n
		}
	}
	return 0
}
