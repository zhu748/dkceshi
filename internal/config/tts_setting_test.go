package config

import "testing"

func TestTTSEnabledDefaultsAndStoredValue(t *testing.T) {
	store := &Store{cfg: Config{}}
	if !store.TTSEnabled() {
		t.Fatal("tts must default to enabled")
	}
	if got := store.TTSSegmentChars(); got != 160 {
		t.Fatalf("segment_chars=%d want 160", got)
	}
	if !store.TTSMultiAccountEnabled() {
		t.Fatal("multi_account must default to enabled")
	}
	if !store.TTSAccountConcurrencyEnabled() {
		t.Fatal("account_concurrency must default to enabled")
	}

	store.cfg.SetSetting("tts", "enabled", false)
	store.cfg.SetSetting("tts", "segment_chars", 200)
	store.cfg.SetSetting("tts", "multi_account", false)
	store.cfg.SetSetting("tts", "account_concurrency", false)
	if store.TTSEnabled() {
		t.Fatal("a stored false must win over the declared default")
	}
	if got := store.TTSSegmentChars(); got != 200 {
		t.Fatalf("segment_chars=%d want 200", got)
	}
	if store.TTSMultiAccountEnabled() {
		t.Fatal("a stored false must win over the declared default")
	}
	if store.TTSAccountConcurrencyEnabled() {
		t.Fatal("a stored false must win over the declared default")
	}
}
