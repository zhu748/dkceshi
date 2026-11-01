package assistantturn

import "ds2api/internal/sse"

const OutputCodePromptEcho = "upstream_prompt_echo"

// Retain the cause even when the streaming guard already withheld every byte.
// The final safety net also covers callers building a turn without a collector.
func rejectPromptEcho(text, prompt string, cut sse.CutReason) (string, sse.CutReason) {
	if cut == sse.CutPromptEcho || sse.IsPromptEcho(text, prompt) {
		return "", sse.CutPromptEcho
	}
	return text, cut
}

func discardEchoTools(turn *Turn) {
	if turn.ContentCut == sse.CutPromptEcho {
		turn.ToolCalls = nil
		turn.ParsedToolCalls.Calls = nil
	}
}

// A body echo must not discard safe reasoning already delivered.
func rejectReasoningEcho(text, prompt string, cut sse.CutReason) (string, sse.CutReason) {
	if sse.IsPromptEcho(text, prompt) {
		return "", sse.CutPromptEcho
	}
	return text, cut
}
