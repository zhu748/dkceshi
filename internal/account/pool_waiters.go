package account

import "ds2api/internal/config"

type poolWaiter struct {
	ready   chan struct{}
	target  string
	exclude map[string]bool
	filter  AccountFilter
}

// canQueueLocked 判断是否还有排队等待的意义。
//   - target 模式：target 必须存在、未被 exclude、且满足 filter
//   - 轮询模式：队列中至少存在一个未被 exclude、且满足 filter 的账号
//
// 排队等待的是"占用中的账号被释放"，因此并发槽位是否已满不影响是否排队，
// 只要有满足 filter 的候选账号即可。若无任何满足 filter 的候选账号，
// 则不应排队，避免请求无限挂起。
//
// 已达 24h 上限的账号不算候选：它当天不会再恢复，排队等到的只会是跨天重置。
// 全部候选都到上限时这里返回 false，取号侧立即以"没有空闲账号"失败（见
// auth.ErrNoIdleAccount），而不是把请求挂到第二天。
func (p *Pool) canQueueLocked(target string, exclude map[string]bool, filter AccountFilter) bool {
	if p.closed {
		return false
	}
	if p.maxQueueSize <= 0 {
		return false
	}
	if len(p.waiters) >= p.maxQueueSize {
		return false
	}
	if target != "" {
		if exclude[target] || p.isDailyLimitedLocked(target) {
			return false
		}
		acc, ok := p.store.FindAccount(target)
		if !ok {
			return false
		}
		if !acc.IsSchedulable() {
			return false
		}
		if filter != nil && !filter(acc) {
			return false
		}
		return true
	}
	// 轮询模式：确认至少存在一个未被 exclude 且满足 filter 的账号，
	// 否则即使唤醒也无法获取，排队无意义。并发槽位是否已满不影响判断，
	// 因为排队的请求等待的是账号被释放后腾出的槽位。
	dailyChecks := p.acquireChecksLocked()
	found := false
	p.forEachQueueAccount(func(id string) bool {
		if exclude[id] || (dailyChecks.dailyActive && p.dailyCounts[id] >= dailyChecks.dailyMax) {
			return true
		}
		acc, ok := p.store.FindAccount(id)
		if !ok || !acc.IsSchedulable() || (filter != nil && !filter(acc)) {
			return true
		}
		found = true
		return false
	})
	return found
}

// Build the free-account view once per notification, rather than repeating
// store lookups, daily-limit checks and a queue copy for every blocked waiter.
func (p *Pool) notifyWaiterLocked() {
	if p.closed || len(p.waiters) == 0 || (p.globalMaxInflight > 0 && p.currentInUseLocked() >= p.globalMaxInflight) {
		return
	}
	view := waiterAvailability{pool: p, checks: p.acquireChecksLocked()}
	for i, waiter := range p.waiters {
		if !view.matches(waiter) {
			continue
		}
		p.waiters = append(p.waiters[:i], p.waiters[i+1:]...)
		close(waiter.ready)
		return
	}
}

type waiterAvailability struct {
	pool       *Pool
	checks     acquireChecks
	loaded     bool
	candidates []config.Account
	targets    map[string]config.Account
}

func (v *waiterAvailability) matches(waiter *poolWaiter) bool {
	if waiter.target != "" {
		id := waiter.target
		if waiter.exclude[id] || !v.pool.canAcquireIDCheckedLocked(id, v.checks) {
			return false
		}
		if v.targets == nil {
			v.targets = make(map[string]config.Account)
		}
		acc, checked := v.targets[id]
		if !checked {
			acc, _ = v.pool.store.FindAccount(id)
			v.targets[id] = acc
		}
		return acc.Identifier() != "" && acc.IsSchedulable() && (waiter.filter == nil || waiter.filter(acc))
	}
	if !v.loaded {
		v.loaded = true
		v.pool.forEachQueueAccount(func(id string) bool {
			if v.pool.canAcquireIDCheckedLocked(id, v.checks) {
				if acc, ok := v.pool.store.FindAccount(id); ok && acc.IsSchedulable() {
					v.candidates = append(v.candidates, acc)
				}
			}
			return true
		})
	}
	for _, acc := range v.candidates {
		if !waiter.exclude[acc.Identifier()] && (waiter.filter == nil || waiter.filter(acc)) {
			return true
		}
	}
	return false
}

func (p *Pool) removeWaiterLocked(waiter *poolWaiter) bool {
	for i, w := range p.waiters {
		if w != waiter {
			continue
		}
		p.waiters = append(p.waiters[:i], p.waiters[i+1:]...)
		return true
	}
	return false
}

func (p *Pool) drainWaitersLocked() {
	for _, waiter := range p.waiters {
		close(waiter.ready)
	}
	p.waiters = nil
}
