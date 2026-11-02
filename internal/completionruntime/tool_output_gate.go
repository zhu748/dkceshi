package completionruntime

import (
	"ds2api/internal/assistantturn"
	"ds2api/internal/promptcompat"
	"ds2api/internal/textclean"
	"ds2api/internal/toolcall"
	"ds2api/internal/toolstream"
)

// Ordinary prose can stream through the tool sieve even when a catalog or
// tool history is present. Only required/forced turns hold prose: an answer
// without a call violates that policy and must remain replaceable.
// Executable tool batches always wait for shared final validation.
func HoldToolOutput(_ []string, _ any, choice promptcompat.ToolChoicePolicy) bool {
	return choice.IsRequired()
}

// Preserve the stream contract after validation: prose before a call is part
// of the answer, while text after a call is never a fabricated tool result.
func ValidatedToolText(turn assistantturn.Turn, names []string, marker string, stripReferences bool) string {
	if len(turn.ToolCalls) == 0 {
		return turn.Text
	}
	// Final repair may recover DSML or malformed shells that the incremental
	// parser never emitted as calls. Use the original source boundary instead.
	source := toolcall.NormalizeMarkerText(turn.RawText, marker)
	prefix := toolstream.TextBeforeToolBoundary(source)
	return textclean.CleanVisibleOutput(toolstream.FilterVisible(prefix, marker), stripReferences)
}
