package tooldiagnostics

import (
	"log/slog"
	"time"
)

// Hidden reasoning can contain a candidate which the final answer supersedes.
// Discard only a provisional first attempt, never a failure or repair history.
func (r *Recorder) discardCandidateLocked() bool {
	if !r.active || r.record.Pinned || len(r.record.Attempts) != 1 || r.current().CompletedAt != 0 {
		return false
	}
	for _, step := range r.current().Steps {
		if step.Stage != "candidate" {
			return false
		}
	}
	r.cancelPersistLocked()
	r.active = false
	r.record.CompletedAt = time.Now().UnixMilli()
	if err := r.store.discardCandidate(r.record.ID); err != nil {
		slog.Warn("[tool_diagnostics] discard provisional candidate failed", "error", err)
	}
	return true
}

func (s *Store) discardCandidate(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	before := s.state
	kept := make([]Summary, 0, len(s.state.Items))
	for _, item := range s.state.Items {
		if item.ID == id && !item.Pinned && item.CompletedAt == 0 {
			continue
		}
		kept = append(kept, item)
	}
	if len(kept) == len(s.state.Items) {
		return nil
	}
	detailPath := s.detailPath(id)
	s.state.Items = kept
	if err := s.commitLocked([]string{detailPath}); err != nil {
		s.state = before
		return err
	}
	return nil
}
