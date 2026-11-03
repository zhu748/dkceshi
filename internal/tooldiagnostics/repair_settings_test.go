package tooldiagnostics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRepairSettingsPersistWithoutExposingKeyAndSurviveRetentionChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "diag.json")
	s := New(path)
	cfg := RepairSettings{Mode: "custom", BaseURL: "https://repair.example/v1", Model: "repair-model", APIKey: "secret-test-key", TimeoutSeconds: 60, ExtraBody: json.RawMessage(`{"reasoning_effort":"high"}`)}
	if err := s.SetRepairSettings(cfg); err != nil {
		t.Fatal(err)
	}
	public := s.List(Query{})
	raw, err := json.Marshal(public)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), cfg.APIKey) || !public.Settings.Repair.HasAPIKey {
		t.Fatal("API key was exposed or key presence lost")
	}
	if err := s.SetSettings(Settings{Enabled: false, Limit: 50}); err != nil {
		t.Fatal(err)
	}
	s = New(path)
	if got := s.RepairSettings(); got.APIKey != cfg.APIKey || got.Model != cfg.Model || got.TimeoutSeconds != 60 {
		t.Fatalf("repair settings lost on restart: %+v", PublicSettings(Settings{Repair: &got}))
	}
	if s.Enabled() {
		t.Fatal("capture was re-enabled")
	}
	if err := s.Clear("normal"); err != nil {
		t.Fatal(err)
	}
	if s.RepairSettings().APIKey != cfg.APIKey {
		t.Fatal("record cleanup deleted repair settings")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Windows exposes synthetic POSIX mode bits; they do not describe ACLs.
	// Keep persistence and key-redaction checks above on every platform.
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatal("private index permissions are too broad")
	}
}

func TestRepairKeyUpdatesPreserveBlankClearExplicitAndDropOnHostChange(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "diag.json"))
	cfg := RepairSettings{Mode: "custom", BaseURL: "https://one.example/v1", Model: "first", APIKey: "key", TimeoutSeconds: 10}
	if err := s.SetRepairSettings(cfg); err != nil {
		t.Fatal(err)
	}
	cfg.APIKey = ""
	cfg.Model = "second"
	if err := s.SetRepairSettings(cfg); err != nil {
		t.Fatal(err)
	}
	if s.RepairSettings().APIKey != "key" {
		t.Fatal("blank update removed existing key")
	}
	cfg.BaseURL = "https://two.example/v1"
	if err := s.SetRepairSettings(cfg); err != nil {
		t.Fatal(err)
	}
	if s.RepairSettings().APIKey != "" {
		t.Fatal("key was reused for a different host")
	}
	cfg.APIKey = "replacement"
	if err := s.SetRepairSettings(cfg); err != nil {
		t.Fatal(err)
	}
	cfg.APIKey = ""
	cfg.ClearAPIKey = true
	if err := s.SetRepairSettings(cfg); err != nil {
		t.Fatal(err)
	}
	if s.RepairSettings().APIKey != "" {
		t.Fatal("explicit key deletion ignored")
	}
}

func TestRepairEndpointAndExtraParametersAreValidated(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"https://example.test", "https://example.test/v1/chat/completions"},
		{"https://example.test/v1/", "https://example.test/v1/chat/completions"},
		{"http://localhost:1234/v1/chat/completions", "http://localhost:1234/v1/chat/completions"},
	} {
		got, err := RepairEndpoint(tc.input)
		if err != nil || got != tc.want {
			t.Fatalf("endpoint %q: %q %v", tc.input, got, err)
		}
	}
	for _, base := range []string{"file:///tmp/key", "https://user:key@example.test/v1", "https://example.test?api_key=secret", "https://example.test/#secret"} {
		if _, err := RepairEndpoint(base); err == nil {
			t.Fatal("invalid API URL accepted")
		}
	}
	s := New(filepath.Join(t.TempDir(), "diag.json"))
	for _, raw := range []string{`[]`, `null`, `{"messages":[]}`, `{"model":"override"}`, `{"stream":true}`, `{"tools":[]}`, `{"parent_message_id":1}`} {
		if err := s.SetRepairSettings(RepairSettings{Mode: "custom", BaseURL: "https://example.test/v1", Model: "test", ExtraBody: json.RawMessage(raw)}); err == nil {
			t.Fatalf("context/protocol override accepted: %s", raw)
		}
	}
	if s.RepairSettings().Mode != "original" {
		t.Fatal("invalid save changed runtime settings")
	}
}

func TestNativeRepairEndpointsAndProtocolCredentialScope(t *testing.T) {
	for _, tc := range []struct{ protocol, base, want string }{
		{"claude", "https://example.test/anthropic", "https://example.test/anthropic/v1/messages"},
		{"claude", "https://example.test/v1/messages", "https://example.test/v1/messages"},
		{"gemini", "https://example.test/v1beta", "https://example.test/v1beta/models/new-model:generateContent"},
		{"gemini", "https://example.test/gemini/v1beta/models/old-model:generateContent", "https://example.test/gemini/v1beta/models/new-model:generateContent"},
	} {
		got, err := RepairAPIEndpoint(RepairSettings{Protocol: tc.protocol, BaseURL: tc.base, Model: "new-model"})
		if err != nil || got != tc.want {
			t.Fatalf("%s endpoint: %q %v", tc.protocol, got, err)
		}
	}
	s := New(filepath.Join(t.TempDir(), "diag.json"))
	if err := s.SetRepairSettings(RepairSettings{Mode: "custom", Protocol: "openai", BaseURL: "https://example.test/v1", Model: "a", APIKey: "saved-key"}); err != nil {
		t.Fatal(err)
	}
	preview, err := s.PreviewRepairSettings(RepairSettings{Protocol: "openai", BaseURL: "https://example.test/v1"})
	if err != nil || preview.APIKey != "saved-key" {
		t.Fatal("discovery did not reuse matching saved credential")
	}
	preview, err = s.PreviewRepairSettings(RepairSettings{Protocol: "claude", BaseURL: "https://example.test/v1"})
	if err != nil || preview.APIKey != "" {
		t.Fatal("saved key crossed API protocols")
	}
	if s.RepairSettings().APIKey != "saved-key" || s.RepairSettings().Protocol != "openai" {
		t.Fatal("discovery preview changed saved configuration")
	}
	for _, raw := range []string{`{"contents":[]}`, `{"system_instruction":{}}`, `{"cached_content":"private"}`, `{"generationConfig":{"candidateCount":2}}`, `{"generation_config":{"candidate_count":2}}`} {
		if err := s.SetRepairSettings(RepairSettings{Mode: "custom", Protocol: "gemini", BaseURL: "https://example.test/v1beta", Model: "model", ExtraBody: json.RawMessage(raw)}); err == nil {
			t.Fatalf("unsafe native extra parameters accepted: %s", raw)
		}
	}
}
