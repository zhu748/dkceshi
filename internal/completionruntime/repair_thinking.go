package completionruntime

import (
	"encoding/json"
	"strings"

	"ds2api/internal/tooldiagnostics"
)

// Explicit panel controls win over advanced JSON. Leaving effort empty keeps
// provider defaults and existing advanced settings exactly as configured.
func applyRepairThinking(payload map[string]any, settings tooldiagnostics.RepairSettings) {
	effort := settings.ReasoningEffort
	if effort == "" {
		return
	}
	budget := map[string]int{"low": 1024, "medium": 4096, "high": 8192}[effort]
	model := strings.ToLower(settings.Model)
	switch tooldiagnostics.RepairProtocol(settings.Protocol) {
	case "claude":
		legacy := strings.Contains(model, "claude-3") || strings.Contains(model, "-4-5") || strings.Contains(model, "-4-1") || strings.Contains(model, "-4-2025")
		if settings.ThinkingMode == "budget" || settings.ThinkingMode == "" && legacy {
			payload["thinking"] = map[string]any{"type": "enabled", "budget_tokens": budget}
			if maxTokens, ok := repairInteger(payload["max_tokens"]); !ok || maxTokens <= budget {
				payload["max_tokens"] = budget + 8192
			}
		} else {
			payload["thinking"] = map[string]any{"type": "adaptive"}
			config := repairObject(payload["output_config"])
			config["effort"] = effort
			payload["output_config"] = config
		}
	case "gemini":
		generation := repairObject(payload["generationConfig"])
		thinking := repairObject(generation["thinkingConfig"])
		if len(thinking) == 0 {
			thinking = repairObject(generation["thinking_config"])
		}
		delete(generation, "thinking_config")
		for _, key := range []string{"thinkingBudget", "thinking_budget", "thinkingLevel", "thinking_level"} {
			delete(thinking, key)
		}
		if settings.ThinkingMode == "budget" || settings.ThinkingMode == "" && strings.Contains(model, "gemini-2.5") {
			thinking["thinkingBudget"] = budget
			if maxTokens, ok := repairInteger(generation["maxOutputTokens"]); !ok || maxTokens <= budget {
				generation["maxOutputTokens"] = budget + 8192
			}
		} else {
			thinking["thinkingLevel"] = effort
		}
		generation["thinkingConfig"] = thinking
		payload["generationConfig"] = generation
	default:
		payload["reasoning_effort"] = effort
	}
}

func repairObject(value any) map[string]any {
	if object, ok := value.(map[string]any); ok {
		return object
	}
	if raw, ok := value.(json.RawMessage); ok {
		var object map[string]any
		if json.Unmarshal(raw, &object) == nil && object != nil {
			return object
		}
	}
	return map[string]any{}
}

func repairInteger(value any) (int, bool) {
	if number, ok := value.(int); ok {
		return number, true
	}
	if raw, ok := value.(json.RawMessage); ok {
		var number int
		if json.Unmarshal(raw, &number) == nil {
			return number, true
		}
	}
	return 0, false
}
