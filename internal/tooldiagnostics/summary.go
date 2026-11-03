package tooldiagnostics

// GetSummary reads committed metadata without opening or decoding a detail.
func (s *Store) GetSummary(id string) (Summary, error) {
	summary, err := indexedSummary(s.stateCache.Load(), id)
	summary.ToolNames = append([]string{}, summary.ToolNames...)
	return summary, err
}

func indexedSummary(index *Index, id string) (Summary, error) {
	if index == nil || !validID(id) {
		return Summary{}, ErrNotFound
	}
	for _, summary := range index.Items {
		if summary.ID == id && validDetailVersion(summary.DetailVersion) {
			return summary, nil
		}
	}
	return Summary{}, ErrNotFound
}
