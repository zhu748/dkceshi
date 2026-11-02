package tooldiagnostics

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEndpointGatewayAndCompletePaths(t *testing.T) {
	for _, tc := range []struct{ protocol, base, generation, models string }{
		{"openai", "/gateway/models/v1/chat/completions", "/gateway/models/v1/chat/completions", "/gateway/models/v1/models"},
		{"openai", "/models", "/models/chat/completions", "/models/models"},
		{"openai", "/messages/chat/completions", "/messages/chat/completions", "/messages/models"},
		{"openai", "/chat/completions", "/chat/completions", "/models"},
		{"claude", "/gateway/models/v1/messages", "/gateway/models/v1/messages", "/gateway/models/v1/models"},
		{"claude", "/gateway/messages", "/gateway/messages", "/gateway/models"},
		{"gemini", "/gateway/models/v1beta/models/old:generateContent", "/gateway/models/v1beta/models/new:generateContent", "/gateway/models/v1beta/models"},
		{"gemini", "/gateway/models/old:generateContent", "/gateway/models/new:generateContent", "/gateway/models"},
		{"gemini", "/v1beta/models", "/v1beta/models/new:generateContent", "/v1beta/models"},
	} {
		cfg := RepairSettings{Protocol: tc.protocol, BaseURL: "https://example.test" + tc.base, Model: "new"}
		got, err := RepairAPIEndpoint(cfg)
		if err != nil || got != "https://example.test"+tc.generation {
			t.Fatalf("%+v: %q %v", tc, got, err)
		}
		got, err = RepairModelsEndpoint(cfg)
		if err != nil || got != "https://example.test"+tc.models {
			t.Fatalf("models: %q %v", got, err)
		}
	}
}
func TestIntermediateFlushAndCancelledTimer(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "diag.json"))
	r := NewRecorder(s, "chat", true)
	ctx := WithRecorder(context.Background(), r)
	AddStep(ctx, Step{Stage: "llm_repair", Result: "running"})
	AddStep(ctx, Step{Stage: "repair_request", Result: "success"})
	Failure(ctx, "repair_failed", "bad output")
	failed, err := s.Get(r.record.ID)
	if err != nil || len(failed.Attempts[0].Steps) != 2 || !failed.Pinned {
		t.Fatalf("lost queued failure evidence: %+v %v", failed, err)
	}
	Failure(ctx, "repair_failed", "bad output")
	again, err := s.Get(r.record.ID)
	if err != nil || again.Revision != failed.Revision {
		t.Fatal("duplicate failure wrote another checkpoint")
	}
	r.Finish(502, nil, false)
	if err := s.Delete(r.record.ID); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * recorderFlushInterval)
	if s.List(Query{}).Total != 0 {
		t.Fatal("timer resurrected deleted record")
	}
}
func TestIntermediateFlushWithoutFinish(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "diag.json"))
	r := NewRecorder(s, "chat", true)
	defer r.Finish(502, nil, false)
	ctx := WithRecorder(context.Background(), r)
	AddStep(ctx, Step{Stage: "llm_repair", Result: "running"})
	AddStep(ctx, Step{Stage: "repair_request", Result: "running"})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		record, err := s.Get(r.record.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(record.Attempts[0].Steps) == 2 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("queued stage never persisted")
}
func TestLateAttemptCannotOverwriteNewGeneration(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "diag.json"))
	r := NewRecorder(s, "chat", true)
	ctx := WithRecorder(context.Background(), r)
	old := WithAttempt(ctx, 1)
	Observe(old, Observation{Relevant: true, Raw: "first", ErrorCode: "invalid_tool_arguments", Error: "first failed"})
	SelectAttempt(ctx, 2, "retry", "new-account")
	current := WithAttempt(ctx, 2)
	Configure(current, "new-model", "new-caller", "new-account", true, nil, nil)
	Observe(current, Observation{Relevant: true, Model: "new-model", Raw: "second", Calls: []Call{{Name: "new-tool", Delivery: "not_delivered"}}})
	Configure(old, "old-model", "old-caller", "old-account", true, nil, nil)
	AddStep(old, Step{Stage: "llm_repair", Result: "failed"})
	Observe(old, Observation{Relevant: true, Raw: "late first", ErrorCode: "late"})
	Failure(old, "late_failure", "timeout")
	RetryDecision(old, "retry_exhausted", "old")
	NextAttempt(old, "regeneration", "late", "old", "old-account")
	r.Finish(200, nil, false)
	record, err := s.Get(r.record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Attempts) != 2 || record.Status != "regenerated_success" || record.CallerID != "new-caller" {
		t.Fatalf("late callback changed request: %+v", record)
	}
	last := record.Attempts[1]
	if last.Raw.Text != "second" || last.Result != "success" || last.AccountID != "new-account" || last.RetryDecision != "" {
		t.Fatalf("late callback changed attempt: %+v", last)
	}
	for _, step := range last.Steps {
		if step.Stage == "llm_repair" {
			t.Fatal("late repair leaked")
		}
	}
}
func TestRetryClosesUnobservedAttemptWithoutRecordingOrdinaryRequests(t *testing.T) {
	for _, relevant := range []bool{false, true} {
		s := New(filepath.Join(t.TempDir(), "diag.json"))
		r := NewRecorder(s, "chat", true)
		ctx := WithRecorder(context.Background(), r)
		NextAttempt(ctx, "regeneration", "upstream_empty_output", "empty first answer", "account")
		Observe(ctx, Observation{Relevant: relevant, Raw: "final"})
		r.Finish(200, nil, false)
		if !relevant {
			if s.List(Query{}).Total != 0 {
				t.Fatal("ordinary retry created record")
			}
			continue
		}
		record, err := s.Get(r.record.ID)
		if err != nil {
			t.Fatal(err)
		}
		a := record.Attempts[0]
		if !record.Pinned || a.Result != "failed" || a.CompletedAt == 0 || a.ErrorCode != "upstream_empty_output" {
			t.Fatalf("running completed attempt: %+v", record)
		}
	}
}
func blockIndex(t *testing.T, path string) func() {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	return func() {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
func TestFailedIndexCommitPreservesRetainedRecordsAndRevision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "diag.json")
	s := New(path)
	for i := 1; i <= 20; i++ {
		if err := s.Put(Record{Summary: Summary{ID: fmt.Sprintf("tool_%02d", i), CreatedAt: int64(i), CompletedAt: 1}}); err != nil {
			t.Fatal(err)
		}
	}
	before := s.List(Query{})
	restore := blockIndex(t, path)
	latest := Record{Summary: Summary{ID: "tool_latest", CreatedAt: 21, CompletedAt: 1}}
	if err := s.Put(latest); err == nil {
		t.Fatal("blocked save accepted")
	}
	after := s.List(Query{})
	if after.Total != before.Total || after.Revision != before.Revision || after.Items[19].ID != "tool_01" || after.StorageError == "" {
		t.Fatalf("failed commit evicted record: %+v", after)
	}
	if _, err := s.Get("tool_01"); err != nil {
		t.Fatal(err)
	}
	restore()
	if err := s.Put(latest); err != nil {
		t.Fatal(err)
	}
	page := New(path).List(Query{})
	if page.Total != 20 || page.Items[0].ID != latest.ID || page.StorageError != "" {
		t.Fatalf("retry failed: %+v", page)
	}
}
func TestFailedSettingsSavePreservesRevision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "diag.json")
	s := New(path)
	before, revision := s.SettingsSnapshot()
	restore := blockIndex(t, path)
	defer restore()
	if err := s.SetRepairSettings(RepairSettings{Mode: "original", TimeoutSeconds: 60}); err == nil {
		t.Fatal("blocked save accepted")
	}
	after, next := s.SettingsSnapshot()
	if next != revision || before.Repair.TimeoutSeconds != after.Repair.TimeoutSeconds {
		t.Fatal("failed save changed settings/revision")
	}
}

func TestFinishClosesAnAttemptBeforeFinalValidation(t *testing.T) {
	for _, stopped := range []bool{false, true} {
		s := New(filepath.Join(t.TempDir(), "diag.json"))
		r := NewRecorder(s, "chat", true)
		ctx := WithRecorder(context.Background(), r)
		AddStep(ctx, Step{Stage: "llm_repair", Result: "running"})
		r.Finish(502, nil, stopped)
		record, err := s.Get(r.record.ID)
		if err != nil {
			t.Fatal(err)
		}
		want := "failed"
		if stopped {
			want = "interrupted"
		}
		if record.Attempts[0].Result != want || record.Attempts[0].CompletedAt == 0 {
			t.Fatalf("ended request kept a running attempt: %+v", record)
		}
	}
}
