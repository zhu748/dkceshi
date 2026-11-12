package assistantturn

import (
	"ds2api/internal/sse"
	"ds2api/internal/tokenusage"
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
	return buildObservedUsage(opts, thinking, text, turn.UpstreamUsage, turn.UpstreamError == "" && turn.ContentCut == sse.CutNone)
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
	return ResolveUsage(buildUsageWithPrefill(BuildOptions{Model: model, Prompt: prompt, RefFileTokens: refFileTokens}, thinking, text), tokenusage.Snapshot{}, true)
}

// BuildUsageFromUpstream avoids tokenizing full prompts/output when the upstream
// has already supplied authoritative input and output counts.
func BuildUsageFromUpstream(model, prompt, thinking, text string, refFileTokens int, observed tokenusage.Snapshot, complete bool) Usage {
	return buildObservedUsage(BuildOptions{Model: model, Prompt: prompt, RefFileTokens: refFileTokens}, thinking, text, observed, complete)
}
func buildObservedUsage(opts BuildOptions, thinking, text string, observed tokenusage.Snapshot, complete bool) Usage {
	if o, ok := observed.Effective(); ok && o.InputTokens != nil && o.OutputTokens != nil {
		sum, sumOK := tokenusage.Count(int64(*o.InputTokens) + int64(*o.OutputTokens))
		if sumOK && (o.TotalTokens == nil || *o.TotalTokens == sum) {
			reasoning := 0
			if o.ReasoningTokens == nil {
				reasoning = util.CountOutputTokens(thinking, opts.Model)
			}
			return ResolveUsage(Usage{ReasoningTokens: reasoning}, observed, complete)
		}
	}
	return ResolveUsage(buildUsageWithPrefill(opts, thinking, text), observed, complete)
}
