package responses

import "time"

type responseStoreLimits struct{ entries, bytes, callerEntries int }

// Called with the store lock held. A caller quota prevents a busy agent from
// consuming the whole entry budget; existing response lifetimes stay unchanged.
func (s *responseStore) configureLimits(store any) {
	reader, ok := store.(interface{ SettingInt(string, string) int })
	if !ok {
		return
	}
	read := func(field string, fallback int) int {
		if n := reader.SettingInt("responses", field); n > 0 {
			return n
		}
		return fallback
	}
	s.limits = responseStoreLimits{entries: read("max_entries", 256), bytes: read("max_bytes_mib", 128) << 20, callerEntries: read("max_caller_entries", 128)}
}

func (s *responseStore) callerCountLocked(owner string) int {
	n := 0
	for _, item := range s.items {
		if item.Owner == owner {
			n++
		}
	}
	return n
}

func (s *responseStore) evictOldestLocked(owner string) {
	oldest := ""
	var expires time.Time
	for key, item := range s.items {
		if owner != "" && item.Owner != owner {
			continue
		}
		if oldest == "" || item.ExpiresAt.Before(expires) {
			oldest, expires = key, item.ExpiresAt
		}
	}
	if oldest != "" {
		s.bytes -= s.items[oldest].Size
		delete(s.items, oldest)
	}
}
