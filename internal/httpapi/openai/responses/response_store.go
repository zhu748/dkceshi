package responses

import (
	"ds2api/internal/jsonvalue"
	"encoding/json"
	"sync"
	"time"

	"ds2api/internal/auth"
)

type storedResponse struct {
	Owner     string
	Value     json.RawMessage
	ExpiresAt time.Time
	Context   json.RawMessage
	Size      int
}

type responseStore struct {
	mu     sync.Mutex
	ttl    time.Duration
	items  map[string]storedResponse
	bytes  int
	limits responseStoreLimits
}

func newResponseStore(ttl time.Duration) *responseStore {
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	return &responseStore{
		ttl:    ttl,
		items:  make(map[string]storedResponse),
		limits: responseStoreLimits{entries: 256, bytes: 128 << 20, callerEntries: 256},
	}
}

func responseStoreKey(owner, id string) string {
	return owner + "\x00" + id
}

func responseStoreOwner(a *auth.RequestAuth) string {
	if a == nil {
		return ""
	}
	return a.CallerID
}

func (s *responseStore) put(owner, id string, value map[string]any) bool {
	if s == nil || owner == "" || id == "" || value == nil {
		return false
	}
	return s.putContext(owner, id, value, nil)
}

func (s *responseStore) putContext(owner, id string, value map[string]any, context []any) bool {
	if s == nil || owner == "" || id == "" || value == nil {
		return false
	}
	// Keep immutable JSON snapshots. Encoding failures must not replace a
	// valid response with nil, and reads need only decode their own snapshot.
	valueJSON, err := json.Marshal(value)
	if err != nil {
		return false
	}
	contextJSON, err := json.Marshal(context)
	if err != nil {
		return false
	}
	size := len(valueJSON) + len(contextJSON)
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLocked(now)
	if size > s.limits.bytes {
		return false
	}
	key := responseStoreKey(owner, id)
	if old, ok := s.items[key]; ok {
		s.bytes -= old.Size
		delete(s.items, key)
	}
	for s.callerCountLocked(owner) >= s.limits.callerEntries {
		s.evictOldestLocked(owner)
	}
	for len(s.items) >= s.limits.entries || s.bytes+size > s.limits.bytes {
		s.evictOldestLocked("")
	}
	s.items[key] = storedResponse{Owner: owner, Value: valueJSON, Context: contextJSON, ExpiresAt: now.Add(s.ttl), Size: size}
	s.bytes += size
	return true
}

func (s *responseStore) get(owner, id string) (map[string]any, bool) {
	if s == nil || owner == "" || id == "" {
		return nil, false
	}
	now := time.Now()
	s.mu.Lock()
	s.sweepLocked(now)
	item, ok := s.items[responseStoreKey(owner, id)]
	s.mu.Unlock()
	if !ok {
		return nil, false
	}
	if item.Owner != owner {
		return nil, false
	}
	var value map[string]any
	if err := jsonvalue.Unmarshal(item.Value, &value); err != nil {
		return nil, false
	}
	return value, true
}

func (s *responseStore) sweepLocked(now time.Time) {
	for k, v := range s.items {
		if now.After(v.ExpiresAt) {
			s.bytes -= v.Size
			delete(s.items, k)
		}
	}
}

func cloneValue(in any) any {
	if in == nil {
		return nil
	}
	data, err := json.Marshal(in)
	if err != nil {
		return nil
	}
	var out any
	if err = jsonvalue.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}
func cloneMessages(in []any) []any { out, _ := cloneValue(in).([]any); return out }
func (s *responseStore) context(owner, id string) ([]any, bool) {
	if s == nil || owner == "" || id == "" {
		return nil, false
	}
	s.mu.Lock()
	s.sweepLocked(time.Now())
	item, ok := s.items[responseStoreKey(owner, id)]
	s.mu.Unlock()
	if !ok || item.Owner != owner || string(item.Context) == "null" {
		return nil, false
	}
	var context []any
	if err := jsonvalue.Unmarshal(item.Context, &context); err != nil {
		return nil, false
	}
	return context, true
}

func (h *Handler) getResponseStore() *responseStore {
	if h == nil {
		return nil
	}
	h.responsesMu.Lock()
	defer h.responsesMu.Unlock()
	ttl := 15 * time.Minute
	if h.Store != nil {
		ttl = time.Duration(h.Store.ResponsesStoreTTLSeconds()) * time.Second
	}
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	if h.responses == nil {
		h.responses = newResponseStore(ttl)
	}
	h.responses.mu.Lock()
	h.responses.ttl = ttl
	h.responses.configureLimits(h.Store)
	h.responses.mu.Unlock()
	return h.responses
}
