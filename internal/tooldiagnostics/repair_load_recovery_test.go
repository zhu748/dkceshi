package tooldiagnostics

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestInvalidSavedRepairCanBeCorrectedWithoutLockingDiagnostics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "diag.json")
	s := New(path)
	if err := s.Put(Record{Summary: Summary{ID: "tool_preserved", CompletedAt: 1, Status: "direct_success"}}); err != nil {
		t.Fatal(err)
	}
	const secret = "saved-private-repair-key"
	bad := RepairSettings{Mode: "custom", Protocol: "openai", BaseURL: "https://repair.example/v1", Model: "model", APIKey: secret, TimeoutSeconds: 10, ExtraBody: json.RawMessage(`{"response_format":{"type":"json_object"}}`)}
	s.state.Settings.Repair = &bad
	if err := s.saveIndexLocked(); err != nil {
		t.Fatal(err)
	}
	s = New(path)
	if !s.Enabled() || s.List(Query{}).Settings.RepairError == "" || s.RepairSettings().UnavailableReason == "" {
		t.Fatal("invalid repair did not enter editable suspended state")
	}
	if _, err := s.Get("tool_preserved"); err != nil {
		t.Fatal("existing evidence unavailable", err)
	}
	raw, err := json.Marshal(s.List(Query{}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), secret) {
		t.Fatal("recoverable settings exposed API key")
	}
	if err := s.SetSettings(Settings{Enabled: false, Limit: 50}); err != nil {
		t.Fatal("unrelated capture settings blocked", err)
	}
	if s.RepairSettings().UnavailableReason == "" {
		t.Fatal("capture toggle reactivated invalid repair")
	}
	corrected := bad
	corrected.ExtraBody = nil
	corrected.APIKey = ""
	if err := s.SetRepairSettings(corrected); err != nil {
		t.Fatal("valid correction blocked", err)
	}
	s = New(path)
	if s.RepairSettings().APIKey != secret || s.RepairSettings().UnavailableReason != "" || s.List(Query{}).Settings.RepairError != "" || s.Enabled() {
		t.Fatal("correction lost secret, warning or capture state")
	}
}
