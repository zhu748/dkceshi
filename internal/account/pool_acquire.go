package account

import (
	"context"

	"ds2api/internal/config"
)

// AccountFilter 决定某个账号是否可被当前请求调度。
// nil 表示不做过滤（兼容旧调用方）。
type AccountFilter func(config.Account) bool

// Acquire 非阻塞获取一个满足 filter 的账号。
func (p *Pool) Acquire(target string, exclude map[string]bool, filter AccountFilter) (config.Account, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.acquireLocked(target, exclude, filter)
}

// AcquireKeepAlive 获取指定账号用于后台保活请求。
// 仅检查可调度性与单账号/全局在途并发限制并执行并发占位（inUse++），
// 不记录每分钟频次与 24h 额度，也不调整队列轮转顺序。
func (p *Pool) AcquireKeepAlive(target string) (config.Account, bool) {
	if target == "" {
		return config.Account{}, false
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return config.Account{}, false
	}
	acc, ok := p.store.FindAccount(target)
	if !ok || !acc.IsSchedulable() {
		return config.Account{}, false
	}
	if p.inUse[target] >= p.maxInflightPerAccount {
		return config.Account{}, false
	}
	if p.globalMaxInflight > 0 && p.currentInUseLocked() >= p.globalMaxInflight {
		return config.Account{}, false
	}
	p.inUse[target]++
	p.inUseTotal++
	return acc, true
}

// AcquireWait 阻塞等待获取一个满足 filter 的账号。
func (p *Pool) AcquireWait(ctx context.Context, target string, exclude map[string]bool, filter AccountFilter) (config.Account, bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	exclude = normalizeExclude(exclude)
	for {
		if ctx.Err() != nil {
			p.mu.Lock()
			p.notifyWaiterLocked()
			p.mu.Unlock()
			return config.Account{}, false
		}

		p.mu.Lock()
		if acc, ok := p.acquireLocked(target, exclude, filter); ok {
			p.notifyWaiterLocked()
			p.mu.Unlock()
			return acc, true
		}
		if !p.canQueueLocked(target, exclude, filter) {
			p.mu.Unlock()
			return config.Account{}, false
		}
		waiter := &poolWaiter{ready: make(chan struct{}), target: target, exclude: exclude, filter: filter}
		p.waiters = append(p.waiters, waiter)
		p.mu.Unlock()

		select {
		case <-ctx.Done():
			p.mu.Lock()
			p.removeWaiterLocked(waiter)
			p.notifyWaiterLocked()
			p.mu.Unlock()
			return config.Account{}, false
		case <-waiter.ready:
		}
	}
}

func (p *Pool) acquireLocked(target string, exclude map[string]bool, filter AccountFilter) (config.Account, bool) {
	if p.closed {
		return config.Account{}, false
	}
	// A saturated global limit cannot be repaired by scanning other accounts.
	if p.globalMaxInflight > 0 && p.currentInUseLocked() >= p.globalMaxInflight {
		return config.Account{}, false
	}
	// One checks snapshot serves the target probe, the rotation scan and the
	// post-acquire bookkeeping; recomputing them per step re-took the store
	// read lock several times per acquire.
	checks := p.acquireChecksLocked()
	if target != "" {
		if exclude[target] || !p.canAcquireIDCheckedLocked(target, checks) {
			return config.Account{}, false
		}
		acc, ok := p.store.FindAccount(target)
		if !ok {
			return config.Account{}, false
		}
		if !acc.IsSchedulable() {
			return config.Account{}, false
		}
		if filter != nil && !filter(acc) {
			return config.Account{}, false
		}
		p.inUse[target]++
		p.inUseTotal++
		p.recordAccountRequestCheckedLocked(target, checks)
		p.bumpQueue(target)
		return acc, true
	}

	return p.tryAcquire(exclude, filter, checks)
}

func (p *Pool) tryAcquire(exclude map[string]bool, filter AccountFilter, checks acquireChecks) (config.Account, bool) {
	// Per-attempt invariants (store locks, date roll, global saturation) are
	// computed once by the caller; the candidate scan below only does map
	// lookups.
	var acquired config.Account
	var found bool
	p.forEachQueueAccount(func(id string) bool {
		if exclude[id] || !p.canAcquireIDCheckedLocked(id, checks) {
			return true
		}
		acc, ok := p.store.FindAccount(id)
		if !ok {
			return true
		}
		if !acc.IsSchedulable() {
			return true
		}
		if filter != nil && !filter(acc) {
			return true
		}
		p.inUse[id]++
		p.inUseTotal++
		p.recordAccountRequestCheckedLocked(id, checks)
		p.bumpQueue(id)
		acquired, found = acc, true
		return false
	})
	return acquired, found
}

// bumpQueue moves accountID to the logical tail of the rotation ring.
// The common case — the account at the logical head, which is what tryAcquire
// picks first — is an O(1) head advance. It replaces the old
// append(queue[:i], queue[i+1:]...) + append which memmoved the whole slice
// (80KB on a 5000-account pool) on every single acquire.
func (p *Pool) bumpQueue(accountID string) {
	n := len(p.queue)
	if n == 0 {
		return
	}
	head := p.queueHead
	if p.queue[head] == accountID {
		p.queueHead = (head + 1) % n
		return
	}
	// General case (targeted acquire, waiters): find the account in
	// logical order and shift the accounts after it one slot earlier,
	// wrapping around the ring until the logical tail.
	phys := -1
	for k := 1; k < n; k++ {
		idx := (head + k) % n
		if p.queue[idx] == accountID {
			phys = idx
			break
		}
	}
	if phys < 0 {
		return
	}
	for {
		next := phys + 1
		if next == n {
			next = 0
		}
		if next == head {
			// phys is already the logical tail.
			break
		}
		p.queue[phys] = p.queue[next]
		phys = next
	}
	p.queue[phys] = accountID
}

func normalizeExclude(exclude map[string]bool) map[string]bool {
	if exclude == nil {
		return map[string]bool{}
	}
	return exclude
}
