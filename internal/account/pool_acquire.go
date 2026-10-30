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
	if target != "" {
		if exclude[target] || !p.canAcquireIDLocked(target) {
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
		p.recordAccountRequestLocked(target)
		p.bumpQueue(target)
		return acc, true
	}

	return p.tryAcquire(exclude, filter)
}

func (p *Pool) tryAcquire(exclude map[string]bool, filter AccountFilter) (config.Account, bool) {
	for i := 0; i < len(p.queue); i++ {
		id := p.queue[i]
		if exclude[id] || !p.canAcquireIDLocked(id) {
			continue
		}
		acc, ok := p.store.FindAccount(id)
		if !ok {
			continue
		}
		if !acc.IsSchedulable() {
			continue
		}
		if filter != nil && !filter(acc) {
			continue
		}
		p.inUse[id]++
		p.recordAccountRequestLocked(id)
		p.bumpQueue(id)
		return acc, true
	}
	return config.Account{}, false
}

func (p *Pool) bumpQueue(accountID string) {
	for i, id := range p.queue {
		if id != accountID {
			continue
		}
		p.queue = append(p.queue[:i], p.queue[i+1:]...)
		p.queue = append(p.queue, accountID)
		return
	}
}

func normalizeExclude(exclude map[string]bool) map[string]bool {
	if exclude == nil {
		return map[string]bool{}
	}
	return exclude
}
