package account

import (
	"math"
	"time"

	"ds2api/internal/config"
)

func (p *Pool) SetStateChanged(fn func()) {
	p.mu.Lock()
	p.onStateChanged = fn
	p.mu.Unlock()
}

func (p *Pool) scheduleExpiryLocked(accounts []config.Account) {
	if p.expiryTimer != nil {
		p.expiryTimer.Stop()
		p.expiryTimer = nil
	}
	now := float64(time.Now().UnixNano()) / 1e9
	next := float64(0)
	for _, acc := range accounts {
		for _, deadline := range []float64{acc.MutedUntil, acc.CooldownUntil} {
			if expires := math.Ceil(deadline); expires > now && (next == 0 || expires < next) {
				next = expires
			}
		}
	}
	if next > 0 {
		p.expiryTimer = time.AfterFunc(time.Duration((next-now)*float64(time.Second))+time.Millisecond, p.recoverExpiredAccounts)
	}
}

func (p *Pool) recoverExpiredAccounts() {
	p.mu.Lock()
	closed := p.closed
	fn := p.onStateChanged
	p.mu.Unlock()
	if closed {
		return
	}
	if p.store.Snapshot().ElasticPool.Enabled {
		busy := p.InUseIDs()
		if err := p.store.Update(func(c *config.Config) error {
			ReconcileElasticPoolWithBusy(c, busy)
			return nil
		}); err != nil {
			config.Logger.Warn("[account_pool] expired account recovery failed", "error", err)
		}
	}
	p.Reset()
	if fn != nil {
		fn()
	}
}

// Close stops background timers and writes any trailing daily quota changes.
func (p *Pool) Close() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.closed = true
	if p.expiryTimer != nil {
		p.expiryTimer.Stop()
	}
	if p.dailyFlushTimer != nil {
		p.dailyFlushTimer.Stop()
		p.dailyFlushTimer = nil
		p.persistDailyLimitsLocked()
	}
	p.clearRateLimitsLocked()
	p.drainWaitersLocked()
}
