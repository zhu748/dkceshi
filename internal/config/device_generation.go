package config

// DeviceSnapshot detects delete/re-add cycles even when all device fields are
// restored to the same values while an external generator is running.
type DeviceSnapshot struct {
	Account Account
	Version uint64
	Proxy   Proxy
}

func (s *Store) DeviceSnapshot(identifier string) (DeviceSnapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, ok := s.findAccountIndexLocked(identifier)
	if !ok {
		return DeviceSnapshot{}, false
	}
	acc := s.cfg.Accounts[idx].clone()
	if s.deviceVersions == nil {
		s.deviceVersions = make(map[string]uint64)
	}
	if s.deviceVersions[acc.Identifier()] == 0 {
		s.deviceSequence++
		s.deviceVersions[acc.Identifier()] = s.deviceSequence
	}
	return DeviceSnapshot{Account: acc, Version: s.deviceVersions[acc.Identifier()], Proxy: generationProxy(s.cfg, acc)}, true
}

func generationProxy(cfg Config, acc Account) Proxy {
	if acc.NoProxy || acc.ProxyID == "" {
		return Proxy{}
	}
	for _, proxy := range cfg.Proxies {
		proxy = NormalizeProxy(proxy)
		if proxy.ID == acc.ProxyID {
			return proxy
		}
	}
	return Proxy{}
}

func sameGenerationState(a, b Account) bool {
	return SameAndroidDevice(a, b) && a.DeviceID == b.DeviceID &&
		a.DeviceIDType == b.DeviceIDType && a.ProxyID == b.ProxyID && a.NoProxy == b.NoProxy
}

func (s *Store) reconcileDeviceVersions(before, after Config) {
	if s.deviceVersions == nil {
		return // No generation has observed this store yet.
	}
	previous := make(map[string]Account, len(before.Accounts))
	for _, acc := range before.Accounts {
		previous[acc.Identifier()] = acc
	}
	next := make(map[string]uint64, len(after.Accounts))
	for _, acc := range after.Accounts {
		id := acc.Identifier()
		version := s.deviceVersions[id]
		old, exists := previous[id]
		if version == 0 || !exists || !sameGenerationState(old, acc) {
			s.deviceSequence++
			version = s.deviceSequence
		}
		next[id] = version
	}
	s.deviceVersions = next
}
