package completionruntime

import (
	"strings"

	"ds2api/internal/assistantturn"
	"ds2api/internal/promptcompat"
	"ds2api/internal/textclean"
	"ds2api/internal/toolstream"
)

// Tool-capable turns can end in a rejected call after ordinary prose. Hold
// answer text until shared validation completes so the entire attempt can be
// replaced without repeating an answer or executing a partial tool batch.
// Reasoning and keep-alive events may still stream while the answer is held.
func HoldToolOutput(names []string, toolsRaw any, choice promptcompat.ToolChoicePolicy) bool {
	tools, _ := toolsRaw.([]any)
	return len(names) > 0 || len(tools) > 0 || choice.Constrained()
}

// Preserve the stream contract after validation: prose before a call is part
// of the answer, while text after a call is never a fabricated tool result.
func ValidatedToolText(turn assistantturn.Turn, names []string, marker string, stripReferences bool) string {
	if len(turn.ToolCalls) == 0 {
		return turn.Text
	}
	state := toolstream.State{CleanVisible: true, DeferMalformed: true, Marker: marker, StripReferenceMarkers: stripReferences}
	events := toolstream.ProcessChunk(&state, turn.RawText, names)
	events = append(events, toolstream.Flush(&state, names)...)
	var text strings.Builder
	for _, event := range events {
		if len(event.ToolCalls) > 0 || len(event.ToolCallDeltas) > 0 {
			break
		}
		text.WriteString(event.Content)
	}
	return textclean.CleanVisibleOutput(toolstream.FilterVisible(text.String(), marker), stripReferences)
}
