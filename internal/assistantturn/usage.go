package assistantturn

import (
	"ds2api/internal/sse"
	"ds2api/internal/util"
	"strings"
)

// Count the generated source, including tool arguments and hidden reasoning.
// Display cleanup must never make tool-only responses free or double count a
// call recovered from the reasoning channel.
func buildTurnUsage(opts BuildOptions, turn Turn) Usage {
	thinking := turn.RawThinking
	detection := turn.DetectionThinking
	if thinking == "" || strings.Contains(detection, thinking) {
		thinking = detection
	} else if !strings.Contains(thinking, detection) {
		thinking += sse.TrimContinuationOverlap(thinking, detection)
	}
	if thinking == "" {
		thinking = turn.Thinking
	}
	text := turn.RawText
	if text == "" {
		text = turn.Text
	}
	return buildUsageWithPrefill(opts, thinking, text)
}

// buildUsageWithPrefill is BuildUsage with the BuildOptions prompt-count
// prefill channel (see BuildOptions.PromptTokenCountFn).
func buildUsageWithPrefill(opts BuildOptions, thinking, text string) Usage {
	inputTokens := 0
	prefilled := false
	if opts.PromptTokenCountFn != nil {
		if count, ok := opts.PromptTokenCountFn(opts.Prompt); ok {
			inputTokens = count
			prefilled = true
		}
	}
	if !prefilled {
		inputTokens = util.CountPromptTokens(opts.Prompt, opts.Model)
	}
	inputTokens += opts.RefFileTokens
	reasoningTokens := util.CountOutputTokens(thinking, opts.Model)
	outputTokens := reasoningTokens + util.CountOutputTokens(text, opts.Model)
	return Usage{
		InputTokens:     inputTokens,
		OutputTokens:    outputTokens,
		ReasoningTokens: reasoningTokens,
		TotalTokens:     inputTokens + outputTokens,
	}
}

func BuildUsage(model, prompt, thinking, text string, refFileTokens int) Usage {
	inputTokens := util.CountPromptTokens(prompt, model) + refFileTokens
	reasoningTokens := util.CountOutputTokens(thinking, model)
	outputTokens := reasoningTokens + util.CountOutputTokens(text, model)
	return Usage{
		InputTokens:     inputTokens,
		OutputTokens:    outputTokens,
		ReasoningTokens: reasoningTokens,
		TotalTokens:     inputTokens + outputTokens,
	}
}
