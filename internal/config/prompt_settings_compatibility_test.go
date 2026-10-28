package config

import (
	"encoding/json"
	"testing"
)

func TestPromptSettingsConfigRejectsWrongJSONTypes(t *testing.T) {
	for _, raw := range []string{
		`{"bottom_format_injection":{"enabled":"false"}}`,
		`{"bottom_format_injection":{"prompt":123}}`,
		`{"expert_prompt_segment":{"max_chars":"160000"}}`,
	} {
		t.Run(raw, func(t *testing.T) {
			var cfg Config
			if err := json.Unmarshal([]byte(raw), &cfg); err == nil {
				t.Fatal("config-file fields must retain their declared JSON types")
			}
		})
	}
}

func TestPromptSettingsConfigZeroReadsAsReferenceDefault(t *testing.T) {
	var cfg Config
	if err := json.Unmarshal([]byte(`{
		"responses":{"store_ttl_seconds":0},
		"expert_prompt_segment":{"max_chars":0},
		"expert_text_file_inline":{"max_file_bytes":0}
	}`), &cfg); err != nil {
		t.Fatalf("stored zero denotes unset, not an invalid API update: %v", err)
	}
	store := &Store{cfg: cfg}
	for _, tc := range []struct {
		key, field string
		want       int
	}{
		{"responses", "store_ttl_seconds", 900},
		{"expert_prompt_segment", "max_chars", 160000},
		{"expert_text_file_inline", "max_file_bytes", 3 * 1024 * 1024},
	} {
		if got := store.SettingInt(tc.key, tc.field); got != tc.want {
			t.Errorf("%s.%s=%d, want %d", tc.key, tc.field, got, tc.want)
		}
	}
}

func TestPromptSettingsNullAndCaseVariantsRoundTrip(t *testing.T) {
	var cfg Config
	if err := json.Unmarshal([]byte(`{
		"bottom_format_injection":{"Enabled":null,"Prompt":null},
		"input_file":{"Enabled":true,"Min_Chars":37},
		"thinking_injection":{"Prompt":"custom prompt"}
	}`), &cfg); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var restored Config
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	store := &Store{cfg: restored}
	if !store.BottomFormatInjectionEnabled() || !store.CurrentInputFileEnabled() || store.CurrentInputFileMinChars() != 37 || store.ThinkingInjectionPrompt() != "custom prompt" {
		t.Fatalf("prompt settings changed after saving and loading: %s", raw)
	}
}
