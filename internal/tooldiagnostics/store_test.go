package tooldiagnostics

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

func TestRetentionPreservesFailuresAcrossResizeDisableRestartAndClear(t *testing.T) {
	path := filepath.Join(t.TempDir(), "diagnostics.json")
	s := New(path)
	if err := s.SetSettings(Settings{Enabled: true, Limit: 50}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 62; i++ {
		r := Record{Summary: Summary{ID: fmt.Sprintf("tool_%03d", i), CreatedAt: int64(i + 1), CompletedAt: 1, Status: "direct_success", ToolNames: []string{"read"}}}
		if i < 2 {
			r.Pinned = true
			r.Status = "failed"
			r.Attempts = []Attempt{{Raw: Capture("failed evidence")}}
		}
		if err := s.Put(r); err != nil {
			t.Fatal(err)
		}
	}
	if got := s.List(Query{}); got.Total != 52 || got.PinnedCount != 2 {
		t.Fatalf("wrong retention: %+v", got)
	}
	if err := s.SetSettings(Settings{Enabled: false, Limit: 20}); err != nil {
		t.Fatal(err)
	}
	s = New(path)
	if got := s.List(Query{}); got.Total != 22 || got.PinnedCount != 2 || got.Settings.Enabled {
		t.Fatalf("restart lost settings or errors: %+v", got)
	}
	if r, err := s.Get("tool_000"); err != nil || r.Attempts[0].Raw.Text != "failed evidence" {
		t.Fatalf("evidence lost: %+v %v", r, err)
	}
	if NewRecorder(s, "chat", false) != nil {
		t.Fatal("disabled capture created a recorder")
	}
	if err := s.Clear("normal"); err != nil {
		t.Fatal(err)
	}
	if got := s.List(Query{}); got.Total != 2 {
		t.Fatal("ordinary clear removed failures")
	}
	if err := s.Clear("errors"); err != nil {
		t.Fatal(err)
	}
	if got := New(path).List(Query{}); got.Total != 0 {
		t.Fatal("explicit failure clear did not persist")
	}
}

func TestRecorderRetainsFailedAttemptAfterSuccessfulRetry(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "diagnostics.json"))
	r := NewRecorder(s, "responses", true)
	ctx := WithRecorder(context.Background(), r)
	Observe(ctx, Observation{Relevant: true, Model: "test", Raw: "original invalid tool", ErrorCode: "invalid_tool_arguments", Error: "missing path", Calls: []Call{{Ordinal: 1, Name: "edit", Parse: "success", Schema: "failed", Delivery: "not_delivered"}}})
	NextAttempt(ctx, "regeneration", "invalid_tool_arguments", "missing path", "account-2")
	Observe(ctx, Observation{Relevant: true, Model: "test", Raw: "valid tool", Calls: []Call{{Ordinal: 1, Name: "edit", Parse: "success", Schema: "passed", Delivery: "not_delivered"}}})
	r.Finish(200, nil, false)
	list := s.List(Query{})
	record, err := s.Get(list.Items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !record.Pinned || record.Status != "regenerated_success" || len(record.Attempts) != 2 {
		t.Fatalf("retry overwrote failure: %+v", record)
	}
	if record.Attempts[0].Raw.Text != "original invalid tool" || record.Attempts[0].Calls[0].Delivery != "not_delivered" || record.Attempts[1].Calls[0].Delivery != "written" {
		t.Fatalf("wrong evidence/delivery: %+v", record)
	}
	// A late release/error cannot change a completed record.
	r.Finish(500, nil, false)
	if got, _ := s.Get(record.ID); got.Status != "regenerated_success" {
		t.Fatal("finish was not idempotent")
	}
}

func TestRestartAndConcurrentRequests(t *testing.T) {
	path := filepath.Join(t.TempDir(), "diagnostics.json")
	s := New(path)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := NewRecorder(s, "chat", true)
			ctx := WithRecorder(context.Background(), r)
			Observe(ctx, Observation{Relevant: true, Raw: "unfinished"})
		}()
	}
	wg.Wait()
	reloaded := New(path)
	list := reloaded.List(Query{})
	if list.Total != 8 || list.PinnedCount != 8 {
		t.Fatalf("interrupted evidence lost: %+v", list)
	}
	for _, item := range list.Items {
		if item.Status != "interrupted" || item.CompletedAt == 0 {
			t.Fatalf("stale running record: %+v", item)
		}
	}
}

func TestTransportFailureDoesNotOverwriteToolsAndUnicodeIsBounded(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "diagnostics.json"))
	r := NewRecorder(s, "chat", true)
	ctx := WithRecorder(context.Background(), r)
	Observe(ctx, Observation{Relevant: true, Raw: "raw candidate", Calls: []Call{{Name: "read", Parse: "success"}}})
	TransportFailure(ctx, Observation{Relevant: true, Raw: "release text", ErrorCode: "validation_unavailable", Error: "lost connection"})
	r.Finish(502, nil, false)
	record, err := s.Get(s.List(Query{}).Items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Attempts[0].Raw.Text != "raw candidate" || len(record.Attempts[0].Calls) != 1 || !record.Pinned {
		t.Fatal("transport overwrote diagnostic evidence")
	}
	text := Capture(strings.Repeat("中🙂", MaxTextBytes))
	if !text.Truncated || !utf8.ValidString(text.Text) || len(text.Text) > MaxTextBytes+40 {
		t.Fatal("invalid or unbounded UTF-8 capture")
	}
	if _, err := s.Get("tool_../../etc/passwd"); err == nil {
		t.Fatal("path traversal accepted")
	}
}

func TestTransientDetailWriteFailureDoesNotLatchOrChangeOutcome(t *testing.T) {
	path := filepath.Join(t.TempDir(), "diag.json")
	s := New(path)
	if err := os.WriteFile(path+".d", []byte("blocked directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := NewRecorder(s, "chat", false)
	ctx := WithRecorder(context.Background(), r)
	o := Observation{Relevant: true, Raw: "tool", Calls: []Call{{Name: "read", Parse: "success"}}}
	Observe(ctx, o)
	if s.List(Query{}).StorageError == "" {
		t.Fatal("missing persistence failure health")
	}
	if err := os.Remove(path + ".d"); err != nil {
		t.Fatal(err)
	}
	Observe(ctx, o)
	r.Finish(200, nil, false)
	page := s.List(Query{})
	if page.StorageError != "" || page.Total != 1 || page.Items[0].Status != "direct_success" {
		t.Fatalf("transient I/O failure latched: %+v", page)
	}
}
