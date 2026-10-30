package responses

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"ds2api/internal/config"
)

func TestResponseStoreRejectsInvalidJSONWithoutReplacingValidEntry(t *testing.T) {
	for _, invalid := range []any{math.NaN(), make(chan int)} {
		s := newResponseStore(time.Minute)
		s.putContext("owner", "response", map[string]any{"id": "original"}, []any{"original context"})
		s.putContext("owner", "response", map[string]any{"bad": invalid}, []any{"new context"})
		got, ok := s.get("owner", "response")
		if !ok || got["id"] != "original" {
			t.Fatalf("invalid JSON destroyed a valid cached response: %#v", got)
		}
		s.putContext("owner", "response", map[string]any{"id": "new"}, []any{invalid})
		context, ok := s.context("owner", "response")
		if !ok || len(context) != 1 || context[0] != "original context" {
			t.Fatalf("invalid replay context destroyed valid context: %#v", context)
		}
	}
}

func TestResponseStorePreservesJSONNumberPrecisionAndEmptyContext(t *testing.T) {
	s := newResponseStore(0)
	number := json.Number("900719925474099312345")
	s.putContext("owner", "response", map[string]any{"number": number}, []any{number})
	value, ok := s.get("owner", "response")
	if !ok || value["number"] != number {
		t.Fatalf("number changed: %#v", value)
	}
	context, ok := s.context("owner", "response")
	if !ok || context[0] != number {
		t.Fatalf("context number changed: %#v", context)
	}
	s.putContext("owner", "empty", map[string]any{"id": "empty"}, []any{})
	if context, ok := s.context("owner", "empty"); !ok || context == nil || len(context) != 0 {
		t.Fatal("empty stored context must remain distinct from an unavailable context")
	}
}

func TestResponseStoreUsesUpdatedTTLForNewEntries(t *testing.T) {
	store, _ := newDirectTokenResolver(t)
	h := &Handler{Store: store}
	s := h.getResponseStore()
	s.put("owner", "old", map[string]any{"id": "old"})
	if err := store.Update(func(c *config.Config) error {
		c.SetSetting("responses", "store_ttl_seconds", 30)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if next := h.getResponseStore(); next != s {
		t.Fatal("updating TTL must not discard stored responses")
	}
	s.put("owner", "new", map[string]any{"id": "new"})
	s.mu.Lock()
	oldExpiry := s.items[responseStoreKey("owner", "old")].ExpiresAt
	newExpiry := s.items[responseStoreKey("owner", "new")].ExpiresAt
	s.mu.Unlock()
	if remaining := time.Until(newExpiry); remaining < 29*time.Second || remaining > 31*time.Second {
		t.Fatalf("new entry ignored updated TTL: %s", remaining)
	}
	if time.Until(oldExpiry) < 14*time.Minute {
		t.Fatal("an existing response's expiry unexpectedly changed")
	}
	// Preserve the constructor's fallback even if a custom config reader or
	// internal updater supplies an unset/invalid TTL after initialization.
	if err := store.Update(func(c *config.Config) error {
		c.SetSetting("responses", "store_ttl_seconds", 0)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	h.getResponseStore().put("owner", "fallback", map[string]any{"id": "fallback"})
	s.mu.Lock()
	fallbackExpiry := s.items[responseStoreKey("owner", "fallback")].ExpiresAt
	s.mu.Unlock()
	if time.Until(fallbackExpiry) < 14*time.Minute {
		t.Fatal("invalid updated TTL bypassed the default lifetime")
	}
}

func TestResponseStoreAccountsForReplacementEvictionAndExpiry(t *testing.T) {
	s := newResponseStore(time.Hour)
	for i := range 300 {
		s.putContext("owner", fmt.Sprint(i), map[string]any{"id": i}, []any{"context"})
	}
	s.putContext("owner", "299", map[string]any{"id": "replacement", "text": "longer output"}, []any{})
	s.mu.Lock()
	if len(s.items) != 256 {
		t.Fatalf("entry limit bypassed: %d", len(s.items))
	}
	bytes := 0
	for key, item := range s.items {
		bytes += len(item.Value) + len(item.Context)
		item.ExpiresAt = time.Now().Add(-time.Second)
		s.items[key] = item
	}
	if bytes != s.bytes {
		t.Fatalf("incorrect stored byte count: %d != %d", bytes, s.bytes)
	}
	s.mu.Unlock()
	s.put("owner", "fresh", map[string]any{"id": "fresh"})
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.items[responseStoreKey("owner", "fresh")]
	if len(s.items) != 1 || s.bytes != item.Size || item.Size != len(item.Value)+len(item.Context) {
		t.Fatal("expired entries were not fully reclaimed")
	}
}

func BenchmarkResponseStore(b *testing.B) {
	text := strings.Repeat("中文 context ", 4096)
	value := map[string]any{"id": "response", "output": []any{map[string]any{"text": text}}}
	context := []any{map[string]any{"role": "user", "content": text}, map[string]any{"role": "assistant", "content": text}}
	for _, action := range []string{"put", "get", "context"} {
		b.Run(action, func(b *testing.B) {
			s := newResponseStore(time.Hour)
			s.putContext("owner", "response", value, context)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				switch action {
				case "put":
					s.putContext("owner", "response", value, context)
				case "get":
					if _, ok := s.get("owner", "response"); !ok {
						b.Fatal("response missing")
					}
				case "context":
					if _, ok := s.context("owner", "response"); !ok {
						b.Fatal("context missing")
					}
				}
			}
		})
	}
}
