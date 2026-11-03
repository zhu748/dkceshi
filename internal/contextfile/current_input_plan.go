package contextfile

import (
	"strings"

	"ds2api/internal/promptcompat"
)

type currentInputPlan struct {
	messages                       []any
	historyText, toolsText, prompt string
	toolNames                      []string
}

// Plan without changing message content. Splitting is an input transport mode;
// it does not depend on token savings or the size of the resulting live prompt.
func planCurrentInputContext(req promptcompat.StandardRequest) *currentInputPlan {
	history, live := promptcompat.SplitActiveContext(req.Messages)
	if len(history) == 0 {
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
		// Preserve the established first-task policy: instructions remain live,
		// and a leading instruction block may itself be externalized while the
		// user task stays inline. A lone user input is externalized in full.
		if len(live) > 0 {
			history, live = live, req.Messages
		}
	}
	plan := &currentInputPlan{}
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
