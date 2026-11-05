package mihomo

import (
	"ds2api/internal/config"
)

// A drain reserves the empty-pool window, preventing a new request from racing
// with a restart. Older PoolResetter implementations keep the idle-only policy.
func (m *Manager) prepareReconcile() (func(), bool) {
	drainer, ok := m.pool.(interface {
		PauseForProxyUpdate() (<-chan struct{}, func())
	})
	if !ok {
		return func() {}, !m.poolHasInflight()
	}
	ready, finish := drainer.PauseForProxyUpdate()
	select {
	case <-m.drainStop:
		finish()
		return nil, false
	case <-ready:
		return finish, true
	default:
	}
	m.drainMu.Lock()
	if m.drainPending {
		m.drainMu.Unlock()
		finish()
		return nil, false
	}
	m.drainPending = true
	m.drainMu.Unlock()
	go func() {
		defer finish()
		defer func() { m.drainMu.Lock(); m.drainPending = false; m.drainMu.Unlock() }()
		select {
		case <-m.drainStop:
			return
		case <-ready:
			m.reconcileBindings()
		}
	}()
	return nil, false
}

func eligibleReconcileAccount(c *config.Config, idx int, expected string) bool {
	if idx < 0 {
		return false
	}
	a := c.Accounts[idx]
	return !a.Disabled && !a.Banned && !a.ManualBinding && !a.NoProxy && a.ProxyID == expected
}

func accountProxyInSnapshot(c config.Config, id string) string {
	idx := findAccountIndexLocked(&c, id)
	if idx < 0 {
		return ""
	}
	return c.Accounts[idx].ProxyID
}

// Stop 终止 mihomo 子进程与健康巡检（服务退出时调用）。
func (m *Manager) Stop() {
	if m == nil {
		return
	}
	m.drainOnce.Do(func() {
		if m.drainStop != nil {
			close(m.drainStop)
		}
	})
	m.reconcileMu.Lock()
	defer m.reconcileMu.Unlock()
	m.stopWatcher()
	m.stopProcess()
}
