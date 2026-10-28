package settings

import (
	"testing"

	"ds2api/internal/config"
)

// TestRegisteredTTSSettingUpdate pins the write path the WebUI speech tab uses:
// `{"tts":{"enabled":false}}` lands in the registered setting, and a partial
// update that omits the field leaves it alone.
func TestRegisteredTTSSettingUpdate(t *testing.T) {
	registered, err := coerceRegisteredSettings(map[string]any{
		"tts": map[string]any{"enabled": false},
	})
	if err != nil {
		t.Fatalf("coerce: %v", err)
	}
	cfg := config.Config{}
	applyRegisteredSettings(&cfg, registered)
	if got, ok := cfg.Settings["tts"]["enabled"].(bool); !ok || got {
		t.Fatalf("tts.enabled=%v want false", cfg.Settings["tts"]["enabled"])
	}

	registered, err = coerceRegisteredSettings(map[string]any{
		"tts": map[string]any{},
	})
	if err != nil {
		t.Fatalf("coerce empty: %v", err)
	}
	applyRegisteredSettings(&cfg, registered)
	if got, ok := cfg.Settings["tts"]["enabled"].(bool); !ok || got {
		t.Fatalf("empty update changed tts.enabled: %v", cfg.Settings["tts"]["enabled"])
	}
}

func TestRegisteredTTSSettingRejectsNonBool(t *testing.T) {
	if _, err := coerceRegisteredSettings(map[string]any{
		"tts": map[string]any{"enabled": "not-a-bool"},
	}); err == nil {
		t.Fatal("expected an error for a non-bool tts.enabled")
	}
}
