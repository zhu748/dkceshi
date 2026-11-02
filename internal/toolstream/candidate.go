package toolstream

// HasToolCandidate reports intercepted protocol which is still being captured.
// Display literals and partial ordinary markup do not activate diagnostics.
func (s *State) HasToolCandidate() bool {
	return s != nil && (s.capturing || len(s.pendingToolCalls) > 0)
}
