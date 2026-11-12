package assistantturn

// IncompleteUsage preserves the resolved field sources when archiving a failed
// turn. It does not publish usage for a failure before any generation exists.
func IncompleteUsage(turn Turn) map[string]any {
	if turn.Prompt == "" && turn.RawText == "" && turn.RawThinking == "" && len(turn.UpstreamUsage.Messages) == 0 {
		return nil
	}
	u := turn.Usage
	if u.Metadata != nil {
		metadata := *u.Metadata
		metadata.Complete = false
		u.Metadata = &metadata
	} else {
		u = ResolveUsage(u, turn.UpstreamUsage, false)
	}
	return OpenAIChatUsage(Turn{Usage: u})
}
