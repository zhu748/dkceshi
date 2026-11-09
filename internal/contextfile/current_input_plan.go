package contextfile

import (
	"strings"
	"unicode/utf8"

	"ds2api/internal/promptcompat"
)

type currentInputPlan struct {
	messages                       []any
	historyMessages                []any
	historyText, toolsText, prompt string
	toolNames                      []string
}

// Large tasks and active loops use file transport without splitting their
// dependencies. This bounds inline conversation content, not system rules.
const maxInlineFirstTaskChars = 32 * 1024

// Plan without changing message content. Splitting is an input transport mode;
// it does not depend on token savings or the size of the resulting live prompt.
func planCurrentInputContext(req promptcompat.StandardRequest) *currentInputPlan {
	history, live := promptcompat.SplitActiveContext(req.Messages)
	if largeActiveContext(live) {
		history, live = externalizedActiveContext(req.Messages)
	} else if len(history) == 0 {
		hasUser := false
		live = nil
		for _, raw := range req.Messages {
			msg, _ := raw.(map[string]any)
			role, _ := msg["role"].(string)
			switch strings.ToLower(strings.TrimSpace(role)) {
			case "assistant", "tool", "function":
				return nil // Keep a first task's active tool loop intact.
			case "system", "developer":
				live = append(live, raw)
			case "user":
				hasUser = true
			}
		}
		if !hasUser {
			return nil
		}
		history = req.Messages
		// Keep short first tasks inline with their instructions. Large first
		// tasks and lone user inputs use the file; instructions remain live.
		if len(live) > 0 && !largeFirstUserInput(req.Messages) {
			history, live = live, req.Messages
		}
	}
	plan := &currentInputPlan{}
	plan.historyMessages = history
	plan.toolsText, _ = promptcompat.BuildOpenAIToolsContextTranscript(req.ToolsRaw, req.ToolChoice)
	plan.messages = append([]any{map[string]any{"role": "system", "content": currentInputFilePrompt(plan.toolsText != "")}}, live...)
	plan.prompt, plan.toolNames = req.BuildPromptWithExternalTools(plan.messages, promptcompat.ReadToolResultNeedsCacheGuard(req.Messages))
	plan.historyText = promptcompat.BuildOpenAICurrentInputContextTranscript(history, req.ToolMarker)
	if strings.TrimSpace(plan.historyText) == "" {
		return nil
	}
	plan.historyText = req.WithThinkingInstruction(injectCurrentInputFileContinuation(plan.historyText))
	return plan
}

func largeFirstUserInput(messages []any) bool {
	chars := 0
	for _, raw := range messages {
		msg, _ := raw.(map[string]any)
		if role, _ := msg["role"].(string); strings.EqualFold(strings.TrimSpace(role), "user") {
			chars += utf8.RuneCountInString(promptcompat.NormalizeOpenAIContentForPrompt(msg["content"]))
			if chars > maxInlineFirstTaskChars {
				return true
			}
		}
	}
	return false
}
