package auth

import (
	"math"
	"sync"
	"time"
)

const loginBurst = 10
const loginRefill = 30 * time.Second
const maxLoginPeers = 1024

type loginBudget struct {
	tokens  float64
	updated time.Time
}
type loginLimiter struct {
	mu    sync.Mutex
	peers map[string]loginBudget
}

// Bound both expensive password attempts and limiter memory. Signed session
// verification never enters this path, so normal dashboard polls are unaffected.
func (l *loginLimiter) allow(peer string, now time.Time) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.peers == nil {
		l.peers = make(map[string]loginBudget)
	}
	budget, exists := l.peers[peer]
	if !exists {
		if len(l.peers) >= maxLoginPeers {
			oldest := ""
			var at time.Time
			for key, b := range l.peers {
				if oldest == "" || b.updated.Before(at) {
					oldest, at = key, b.updated
				}
			}
			delete(l.peers, oldest)
		}
		budget = loginBudget{tokens: loginBurst, updated: now}
	}
	budget.tokens = math.Min(loginBurst, budget.tokens+math.Max(0, now.Sub(budget.updated).Seconds())/loginRefill.Seconds())
	budget.updated = now
	if budget.tokens < 1 {
		l.peers[peer] = budget
		return false, max(1, int(math.Ceil((1-budget.tokens)*loginRefill.Seconds())))
	}
	budget.tokens--
	l.peers[peer] = budget
	return true, 0
}

// Successful password authentication restores the budget, so existing clients
// using raw admin credentials are not throttled during normal operation.
func (l *loginLimiter) reset(peer string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.peers, peer)
}
