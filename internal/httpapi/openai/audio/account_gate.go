package audio

import (
	"context"
	"strings"
	"sync"
)

// resolvePerAccountLimit caps concurrent segment generations per account for TTS.
// It is capped at 2 at most even if runtime inflight is higher, and drops to 1 if
// accountConcurrency is disabled or if the global runtime inflight is 1.
func resolvePerAccountLimit(store ConfigReader, accountConcurrency bool) int {
	if !accountConcurrency {
		return 1
	}
	if store != nil && store.RuntimeAccountMaxInflight() == 1 {
		return 1
	}
	return 2
}

// accountGate caps concurrent segment generations per account. The pool's
// in-flight limit is global configuration; this lets the TTS settings switch
// between two generations per account and one (the safer mode when accounts
// are being rate-limited).
type accountGate struct {
	mu       sync.Mutex
	gates    map[string]chan struct{}
	capacity int
}

func newAccountGate(capacity int) *accountGate {
	if capacity < 1 {
		capacity = 1
	}
	return &accountGate{gates: map[string]chan struct{}{}, capacity: capacity}
}

func (g *accountGate) acquire(ctx context.Context, accountID string) (func(), error) {
	key := strings.TrimSpace(accountID)
	if key == "" {
		key = "_"
	}
	g.mu.Lock()
	gate := g.gates[key]
	if gate == nil {
		gate = make(chan struct{}, g.capacity)
		g.gates[key] = gate
	}
	g.mu.Unlock()
	select {
	case gate <- struct{}{}:
		return func() { <-gate }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
