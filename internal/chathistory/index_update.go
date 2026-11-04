package chathistory

// Updates preserve creation time. Only the changed summary and its position
// among equal creation timestamps can change; keep the other previews intact.
func (s *Store) updateSummaryLocked(item Entry) bool {
	index, first := -1, -1
	for i, summary := range s.state.Items {
		if summary.CreatedAt == item.CreatedAt && first < 0 {
			first = i
		}
		if summary.ID == item.ID {
			index = i
			break
		}
	}
	if index < 0 || first < 0 {
		return false
	}
	copy(s.state.Items[first+1:index+1], s.state.Items[first:index])
	s.state.Items[first] = summaryFromEntry(item)
	return true
}

func (s *Store) saveUpdatedDetailLocked(item Entry) error {
	if !s.updateSummaryLocked(item) {
		return s.saveLocked()
	}
	return s.persistLocked()
}
