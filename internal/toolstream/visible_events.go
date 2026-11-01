package toolstream

// ProcessChunk keeps classification and cleanup ahead of every visible delta.
// Deferred protocol source is preserved verbatim for authoritative finalization.
func ProcessChunk(state *State, chunk string, toolNames []string) []Event {
	return cleanVisibleEvents(state, processChunk(state, chunk, toolNames))
}

func Flush(state *State, toolNames []string) []Event {
	events := cleanVisibleEvents(state, flush(state, toolNames))
	if state != nil && state.CleanVisible {
		if tail := state.cleaner.Flush(); tail != "" {
			events = append(events, Event{Content: tail})
		}
	}
	return events
}

func cleanVisibleEvents(state *State, events []Event) []Event {
	if state == nil || !state.CleanVisible {
		return events
	}
	state.cleaner.StripReferenceMarkers = state.StripReferenceMarkers
	for i := range events {
		if !events[i].Deferred && events[i].Content != "" {
			events[i].Content = state.cleaner.Write(events[i].Content)
		}
	}
	return events
}
