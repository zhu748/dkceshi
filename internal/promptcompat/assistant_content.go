package promptcompat

import (
	"strings"

	"ds2api/internal/prompt"
)

// Keep reasoning blocks at their source position when content interleaves
// reasoning and ordinary text. The separate reasoning_content field precedes
// content unless it repeats the exact same reasoning representation.
func assistantContentForPrompt(message map[string]any, marker string, normalizeTools bool) (string, string) {
	blocks, ok := message["content"].([]any)
	if !ok {
		content := prompt.NormalizeAssistantContent(message)
		if normalizeTools {
			content = normalizeAssistantToolMarkupContentForPrompt(content, marker)
		}
		return content, prompt.AssistantReasoningText(message)
	}
	var parts []string
	var run []any
	reasoningRun := false
	flush := func() {
		if len(run) == 0 {
			return
		}
		var text string
		if reasoningRun {
			if text = prompt.ReasoningText(run); text != "" {
				text = formatPromptLabeledBlock(assistantReasoningLabel, text)
			}
		} else {
			text = prompt.NormalizeContent(run)
			if normalizeTools {
				text = normalizeAssistantToolMarkupContentForPrompt(text, marker)
			}
		}
		if text != "" {
			parts = append(parts, text)
		}
		run = nil
	}
	for _, block := range blocks {
		object, _ := block.(map[string]any)
		kind := strings.ToLower(strings.TrimSpace(asString(object["type"])))
		isReasoning := kind == "thinking" || kind == "reasoning"
		if isReasoning != reasoningRun {
			flush()
			reasoningRun = isReasoning
		}
		run = append(run, block)
	}
	flush()
	content := prompt.NormalizeAssistantContent(map[string]any{"content": strings.Join(parts, "\n\n"), "refusal": message["refusal"], "audio": message["audio"]})
	reasoning := prompt.ReasoningText(message["reasoning_content"])
	if reasoning == prompt.ContentReasoningText(blocks) {
		reasoning = ""
	}
	return content, reasoning
}
