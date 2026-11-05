package config

import "slices"

// ProxySnapshot copies only proxy records; import previews do not clone accounts or subscriptions.
func (s *Store) ProxySnapshot() []Proxy {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.cfg.Proxies)
}
