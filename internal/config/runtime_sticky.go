package config

func (s *Store) RuntimeAccountStickyEnabled() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.Runtime.AccountStickyEnabled != nil && *s.cfg.Runtime.AccountStickyEnabled
}
