package config

import (
	"slices"
	"strings"
)

func (s *Store) Snapshot() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.Clone()
}

func (s *Store) HasAPIKey(k string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.keyMap[k]
	return ok
}

// APIKeyFlag reports the value of a registered credential flag for the managed
// API key k. A key that does not set the flag — and an unknown key — reads as
// the flag's declared default, and an unregistered flag name reads as false.
func (s *Store) APIKeyFlag(k, flag string) bool {
	spec, ok := LookupCredentialFlag(flag)
	if !ok {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if flags, ok := s.keyFlags[k]; ok {
		if value, ok := flags[flag]; ok {
			return value
		}
	}
	return spec.Default
}

// APIKeyFlags reports every registered credential flag of the managed API key k,
// each falling back to its declared default, so a caller can read a whole key's
// switches in one pass. The admin key payload reports the same values through
// APIKey.MarshalJSON; this is the direct read path for callers that are not
// rendering a key record.
func (s *Store) APIKeyFlags(k string) map[string]bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	stored := s.keyFlags[k]
	out := make(map[string]bool, len(CredentialFlagSpecs()))
	for _, spec := range CredentialFlagSpecs() {
		if value, ok := stored[spec.Name]; ok {
			out[spec.Name] = value
			continue
		}
		out[spec.Name] = spec.Default
	}
	return out
}

func (s *Store) Keys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.cfg.Keys)
}

func (s *Store) Accounts() []Account {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneAccounts(s.cfg.Accounts)
}

func (s *Store) FindAccount(identifier string) (Account, bool) {
	identifier = strings.TrimSpace(identifier)
	s.mu.RLock()
	defer s.mu.RUnlock()
	if idx, ok := s.findAccountIndexLocked(identifier); ok {
		return s.cfg.Accounts[idx].clone(), true
	}
	return Account{}, false
}
