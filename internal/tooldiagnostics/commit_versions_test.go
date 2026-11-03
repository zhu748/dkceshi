package tooldiagnostics

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFailedUpdateKeepsCommittedDetailAndRetriesFinalCheckpoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "diag.json")
	s := New(path)
	r := NewRecorder(s, "chat", true)
	ctx := WithRecorder(context.Background(), r)
	Candidate(ctx, "content", Capture("original"))
	initial, err := s.Get(r.record.ID)
	if err != nil {
		t.Fatal(err)
	}
	restore := blockIndex(t, path)
	Observe(ctx, Observation{Relevant: true, Raw: "failure evidence", ErrorCode: "invalid_tool_arguments", Error: "invalid"})
	r.Finish(422, nil, false)
	committed, err := s.Get(r.record.ID)
	if err != nil || committed.RawEvidence() != initial.RawEvidence() || committed.Pinned {
		t.Fatalf("uncommitted evidence visible: %+v %v", committed, err)
	}
	restore()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		page := s.List(Query{})
		if page.Items[0].CompletedAt != 0 {
			detail, err := s.Get(r.record.ID)
			if err != nil || detail.Status != "failed" || !detail.Pinned || page.StorageError != "" {
				t.Fatalf("bad recovery: %+v %v", detail, err)
			}
			if err := s.Delete(r.record.ID); err != nil {
				t.Fatal(err)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("final checkpoint did not recover")
}

func (r Record) RawEvidence() string {
	if len(r.Attempts) == 0 {
		return ""
	}
	return r.Attempts[0].Raw.Text
}

func TestLegacyDetailMigrationAndOrphanCleanup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "diag.json")
	s := New(path)
	record := Record{Summary: Summary{ID: "tool_legacy", Status: "direct_success", CompletedAt: 1}}
	if err := atomicJSON(s.versionPath(record.ID, ""), record); err != nil {
		t.Fatal(err)
	}
	s.state.Items = []Summary{record.Summary}
	if err := s.saveIndexLocked(); err != nil {
		t.Fatal(err)
	}
	s = New(path)
	loaded, err := s.Get(record.ID)
	if err != nil || loaded.DetailVersion != "" {
		t.Fatal(err)
	}
	if err := s.Put(loaded); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.versionPath(record.ID, "")); !os.IsNotExist(err) {
		t.Fatalf("legacy detail not migrated: %v", err)
	}
	orphan := s.versionPath("tool_orphan", "f47ac10b-58cc-4372-a567-0e02b2c3d479")
	if err := os.WriteFile(orphan, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	s = New(path)
	if _, err := s.Get(record.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatalf("orphan not collected: %v", err)
	}
}

func TestRepairOptionsDefaultsZeroValidationAndSnapshots(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "diag.json"))
	if RepairRetryCount(s.RepairSettings()) != 3 {
		t.Fatal("missing default retries")
	}
	var draft RepairSettings
	if err := json.Unmarshal([]byte(`{"mode":"custom","base_url":"https://example.test/v1","model":"model","retry_count":0,"reasoning_effort":"high"}`), &draft); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRepairSettings(draft); err != nil {
		t.Fatal(err)
	}
	copy := s.RepairSettings()
	*copy.RetryCount = 9
	if RepairRetryCount(s.RepairSettings()) != 0 {
		t.Fatal("snapshot shares retry pointer")
	}
	if RepairRetryCount(New(s.path).RepairSettings()) != 0 {
		t.Fatal("zero retries lost on restart")
	}
	for _, raw := range []string{`{"functions":[]}`, `{"function_call":"auto"}`, `{"response_format":{"type":"json_object"}}`, `{"generationConfig":{"responseMimeType":"application/json"}}`} {
		if validateRepairExtraBody(json.RawMessage(raw)) == nil {
			t.Fatalf("output override accepted: %s", raw)
		}
	}
	retries := 11
	draft.RetryCount = &retries
	if s.SetRepairSettings(draft) == nil {
		t.Fatal("unbounded retries accepted")
	}
}

func TestCommittedListAndDetailDoNotWaitForWriterLock(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "diag.json"))
	record := Record{Summary: Summary{ID: "tool_snapshot", CompletedAt: 1}, Tools: Capture("catalog")}
	if err := s.Put(record); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	done := make(chan error, 1)
	go func() {
		_, err := s.Get(record.ID)
		page := s.List(Query{})
		if err == nil && (page.Total != 1 || page.StoredTextBytes != 7) {
			err = ErrNotFound
		}
		done <- err
	}()
	select {
	case err := <-done:
		s.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		s.mu.Unlock()
		<-done
		t.Fatal("read waited behind persistence mutex")
	}
}

func TestCommittedSummaryOwnsToolNames(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "diag.json"))
	record := Record{Summary: Summary{ID: "tool_owned_names", CompletedAt: 1, ToolNames: []string{"read"}}}
	if err := s.Put(record); err != nil {
		t.Fatal(err)
	}
	record.ToolNames[0] = "changed by caller"
	if names := s.List(Query{}).Items[0].ToolNames; len(names) != 1 || names[0] != "read" {
		t.Fatalf("committed summary shared caller slice: %v", names)
	}
}
