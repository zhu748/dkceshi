package tooldiagnostics

import (
	"context"
	"time"
)

// Candidate is durable evidence, never a successful parse or permission to
// execute. Final observations replace it; late progress cannot replace them.
func Candidate(ctx context.Context, source string, raw Text) {
	r := FromContext(ctx)
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.matchesAttempt(ctx) || r.record.CompletedAt != 0 || r.current().CompletedAt != 0 {
		return
	}
	first := !r.active
	r.active = true
	a := r.current()
	if a.Source == "" {
		a.Steps = append(a.Steps, Step{Stage: "candidate", Result: "running", At: time.Now().UnixMilli()})
	}
	a.Raw, a.Source = raw, source
	if first {
		r.persistLocked()
	} else {
		r.schedulePersistLocked()
	}
}
