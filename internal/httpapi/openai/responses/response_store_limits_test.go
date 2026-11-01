package responses

import (
	"ds2api/internal/config"
	"fmt"
	"testing"
	"time"
)

func TestBusyAgentEvictsItsOwnQuotaBeforeOtherCallers(t *testing.T) {
	s := newResponseStore(time.Hour)
	s.limits = responseStoreLimits{entries: 4, bytes: 1 << 20, callerEntries: 2}
	s.put("other", "active", map[string]any{"id": "active"})
	for i := range 12 {
		s.put("busy", fmt.Sprint(i), map[string]any{"id": i})
	}
	if _, ok := s.get("other", "active"); !ok {
		t.Fatal("busy caller evicted another caller before exhausting its own quota")
	}
	if _, ok := s.get("busy", "11"); !ok {
		t.Fatal("latest agent response lost")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.items) != 3 || s.callerCountLocked("busy") != 2 {
		t.Fatalf("quota not enforced: %d", len(s.items))
	}
}

func TestResponseCapacitySettingsAreAppliedWithoutDiscardingState(t *testing.T) {
	store, _ := newDirectTokenResolver(t)
	h := &Handler{Store: store}
	s := h.getResponseStore()
	s.put("owner", "active", map[string]any{"id": "active"})
	if err := store.Update(func(c *config.Config) error {
		c.SetSetting("responses", "max_entries", 10)
		c.SetSetting("responses", "max_bytes_mib", 2)
		c.SetSetting("responses", "max_caller_entries", 3)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if h.getResponseStore() != s {
		t.Fatal("settings change replaced the store")
	}
	s.mu.Lock()
	limits := s.limits
	s.mu.Unlock()
	if limits.entries != 10 || limits.bytes != 2<<20 || limits.callerEntries != 3 {
		t.Fatalf("settings not applied: %+v", limits)
	}
	if _, ok := s.get("owner", "active"); !ok {
		t.Fatal("capacity update discarded valid context")
	}
}

func TestOversizedReplacementPreservesExistingResponse(t *testing.T) {
	s := newResponseStore(time.Hour)
	s.limits.bytes = 1024
	s.put("owner", "active", map[string]any{"id": "original"})
	s.put("owner", "active", map[string]any{"large": make([]int, 2048)})
	value, ok := s.get("owner", "active")
	if !ok || value["id"] != "original" {
		t.Fatal("failed capacity check destroyed valid continuation")
	}
}
