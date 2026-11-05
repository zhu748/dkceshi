package proxybridge

import (
	"context"
	"time"
)

func reap() {
	ticker := time.NewTicker(time.Minute)
	for now := range ticker.C {
		reapIdle(now)
	}
}
func reapIdle(now time.Time) {
	var retired []*instance
	registry.Lock()
	for key, e := range registry.items {
		if e.refs.Load() != 0 || !e.mu.TryLock() {
			continue
		}
		if e.stopped() || now.Sub(time.Unix(0, e.last.Load())) > 30*time.Minute {
			delete(registry.items, key)
			if e.cancel != nil {
				e.cancel()
			}
			retired = append(retired, e)
		} else {
			e.mu.Unlock()
		}
	}
	registry.Unlock()
	for _, e := range retired {
		e.stop()
		e.mu.Unlock()
	}
}
func (e *instance) stopped() bool {
	if e.cmd == nil {
		return true
	}
	select {
	case <-e.done:
		return true
	default:
		return false
	}
}
func discardStopped(key string, e *instance) {
	registry.Lock()
	defer registry.Unlock()
	if registry.items[key] != e || e.refs.Load() != 0 || !e.mu.TryLock() {
		return
	}
	defer e.mu.Unlock()
	if e.stopped() {
		e.stop()
		if e.cancel != nil {
			e.cancel()
		}
		delete(registry.items, key)
	}
}
func CloseAll() {
	registry.Lock()
	old := registry.items
	for _, e := range old {
		if e.cancel != nil {
			e.cancel()
		}
	}
	registry.items = map[string]*instance{}
	registry.Unlock()
	for _, e := range old {
		e.mu.Lock()
		e.stop()
		e.mu.Unlock()
	}
}

func (e *instance) lockContext(ctx context.Context) error {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if e.mu.TryLock() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// Called with registry locked; the returned instance stays locked until stopped.
func evictIdleLocked() *instance {
	var oldest *instance
	var oldestKey string
	for key, e := range registry.items {
		if e.refs.Load() != 0 || !e.mu.TryLock() {
			continue
		}
		if oldest == nil || e.last.Load() < oldest.last.Load() {
			if oldest != nil {
				oldest.mu.Unlock()
			}
			oldest, oldestKey = e, key
		} else {
			e.mu.Unlock()
		}
	}
	if oldest != nil {
		delete(registry.items, oldestKey)
		if oldest.cancel != nil {
			oldest.cancel()
		}
	}
	return oldest
}
