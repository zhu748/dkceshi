package account

import "sync"

// PauseForProxyUpdate atomically blocks new leases while existing leases drain.
// Queued requests retain their own cancellation and queue limits.
func (p *Pool) PauseForProxyUpdate() (<-chan struct{}, func()) {
	p.mu.Lock()
	if p.proxyPauses == 0 {
		p.proxyIdle = make(chan struct{})
	}
	p.proxyPauses++
	p.signalProxyIdleLocked()
	ready := p.proxyIdle
	p.mu.Unlock()
	var once sync.Once
	return ready, func() {
		once.Do(func() {
			p.mu.Lock()
			defer p.mu.Unlock()
			p.proxyPauses--
			if p.proxyPauses == 0 {
				p.notifyWaiterLocked()
			}
		})
	}
}

func (p *Pool) signalProxyIdleLocked() {
	if p.proxyPauses == 0 || (!p.closed && p.currentInUseLocked() > 0) {
		return
	}
	select {
	case <-p.proxyIdle:
	default:
		close(p.proxyIdle)
	}
}
