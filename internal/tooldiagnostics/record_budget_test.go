package tooldiagnostics

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRecordBudgetKeepsNewestEvidenceWithoutChangingLiveRecord(t *testing.T) {
	value := Capture(strings.Repeat("字", MaxTextBytes))
	r := Record{Attempts: make([]Attempt, 32)}
	for i := range r.Attempts {
		r.Attempts[i] = Attempt{Raw: value, Residual: value, Calls: []Call{{Name: "read", Arguments: value}}, Steps: []Step{{Input: value, Output: value}}}
	}
	bounded := boundRecord(r)
	if !bounded.EvidenceTruncated || bounded.Attempts[31].Raw.Text != value.Text || r.Attempts[0].Calls[0].Arguments.Text != value.Text {
		t.Fatal("budget mutated source or lost newest evidence")
	}
	total := len(bounded.Tools.Text) + len(bounded.Policy.Text)
	for _, a := range bounded.Attempts {
		total += len(a.Raw.Text) + len(a.Residual.Text)
		for _, c := range a.Calls {
			total += len(c.Arguments.Text)
		}
		for _, s := range a.Steps {
			total += len(s.Input.Text) + len(s.Output.Text)
		}
	}
	if total > MaxRecordTextBytes || bounded.Attempts[0].Raw.Bytes != value.Bytes || !bounded.Attempts[0].Raw.Truncated {
		t.Fatal("aggregate evidence budget failed")
	}
}

func TestCompletionSettingsDoNotWaitForStorageLock(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "diag.json"))
	if err := s.SetRepairSettings(RepairSettings{Mode: "original", Model: "saved", TimeoutSeconds: 20}); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	done := make(chan bool, 1)
	go func() { done <- s.Enabled() && s.RepairSettings().TimeoutSeconds == 20 }()
	select {
	case ok := <-done:
		if !ok {
			t.Fatal("stale settings")
		}
	case <-time.After(time.Second):
		t.Fatal("completion setup waited behind disk storage")
	}
}
