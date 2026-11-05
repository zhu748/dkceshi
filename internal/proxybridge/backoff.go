package proxybridge

import "time"

type startFailure struct {
	err   error
	until time.Time
}

// Cache only failed starts, independently of live instance capacity. Different
// credentials/configuration produce a different key and bypass the old failure.
func cachedStartFailure(key string) error {
	registry.Lock()
	defer registry.Unlock()
	f := registry.failures[key]
	if time.Now().Before(f.until) {
		return f.err
	}
	delete(registry.failures, key)
	return nil
}

func rememberStartFailure(key string, err error) {
	registry.Lock()
	defer registry.Unlock()
	if registry.failures == nil {
		registry.failures = map[string]startFailure{}
	}
	for key, f := range registry.failures {
		if !time.Now().Before(f.until) {
			delete(registry.failures, key)
		}
	}
	if len(registry.failures) < 128 {
		registry.failures[key] = startFailure{err, time.Now().Add(3 * time.Second)}
	}
}
