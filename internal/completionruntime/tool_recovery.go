package completionruntime

import "ds2api/internal/assistantturn"

// ToolRecovery holds replacement prose after a rejected batch with a delivered
// preamble. Only the validated tools are appended; a genuine text-only answer
// is still delivered. Keep this state across multiple failed generations.
type ToolRecovery struct{ Hold bool }

func (r *ToolRecovery) Start(code string, textDelivered bool) {
	r.Hold = r.Hold || textDelivered && IsToolOutputError(&assistantturn.OutputError{Code: code})
}

func (r ToolRecovery) Text(turn assistantturn.Turn, names []string, marker string, stripReferences bool) string {
	if r.Hold && len(turn.ToolCalls) > 0 {
		return ""
	}
	return ValidatedToolText(turn, names, marker, stripReferences)
}
