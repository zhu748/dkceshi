package tooldiagnostics

import "context"

type attemptContextKey struct{}

// Bind asynchronous callbacks to the generation that produced their evidence.
func WithAttempt(ctx context.Context, number int) context.Context {
	return context.WithValue(ctx, attemptContextKey{}, number)
}

// Call with r.mu held so checking and applying an update cannot race.
func (r *Recorder) matchesAttempt(ctx context.Context) bool {
	number, bound := ctx.Value(attemptContextKey{}).(int)
	return !bound || number == r.number
}
func HasObservation(ctx context.Context) bool {
	r := FromContext(ctx)
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.matchesAttempt(ctx) && r.current().CompletedAt != 0
}
