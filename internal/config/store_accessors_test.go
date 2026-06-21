package config

import "testing"

func TestStoreCurrentInputFileAccessors(t *testing.T) {
	store := &Store{cfg: Config{}}
	// 默认关闭：历史拆分（current_input_file）需要用户显式开启，
	// 或通过 -forcehistory 模型后缀按需触发。
	if store.CurrentInputFileEnabled() {
		t.Fatal("expected current input file disabled by default")
	}
	if got := store.CurrentInputFileMinChars(); got != 0 {
		t.Fatalf("default current input file min_chars=%d want=0", got)
	}

	enabled := false
	store.cfg.CurrentInputFile = CurrentInputFileConfig{Enabled: &enabled, MinChars: 12345}
	if store.CurrentInputFileEnabled() {
		t.Fatal("expected current input file disabled")
	}

	enabled = true
	store.cfg.CurrentInputFile.Enabled = &enabled
	if !store.CurrentInputFileEnabled() {
		t.Fatal("expected current input file enabled")
	}
	if got := store.CurrentInputFileMinChars(); got != 12345 {
		t.Fatalf("current input file min_chars=%d want=12345", got)
	}
}

func TestStoreThinkingInjectionAccessors(t *testing.T) {
	store := &Store{cfg: Config{}}
	// 默认关闭：思考注入应按需启用，避免对所有请求一律注入提示词。
	// 启用方式：config 中显式 enabled=true，或使用 -thinkinginject 模型后缀。
	if store.ThinkingInjectionEnabled() {
		t.Fatal("expected thinking injection disabled by default")
	}

	enabled := true
	store.cfg.ThinkingInjection.Enabled = &enabled
	if !store.ThinkingInjectionEnabled() {
		t.Fatal("expected thinking injection enabled by explicit config")
	}

	store.cfg.ThinkingInjection.Prompt = "  custom thinking prompt  "
	if got := store.ThinkingInjectionPrompt(); got != "custom thinking prompt" {
		t.Fatalf("thinking injection prompt=%q want custom thinking prompt", got)
	}
}
