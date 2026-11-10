package responsehistory

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"ds2api/internal/auth"
	"ds2api/internal/chathistory"
	"ds2api/internal/promptcompat"
	"ds2api/internal/usagestats"
)

func TestProgressPersistDueCoalescesFrequentUpdates(t *testing.T) {
	base := time.Unix(100, 0)
	if progressPersistDue(base.Add(2*time.Second), base) {
		t.Fatal("expected frequent progress update to be coalesced")
	}
	if !progressPersistDue(base.Add(progressPersistInterval), base) {
		t.Fatal("expected progress update after the persist interval")
	}
}

func TestProgressPersistDueAllowsInitialUpdate(t *testing.T) {
	if !progressPersistDue(time.Unix(100, 0), time.Time{}) {
		t.Fatal("expected initial progress update")
	}
}

func TestStartRecordsUsageWithoutHistoryStore(t *testing.T) {
	usageStore := usagestats.New(filepath.Join(t.TempDir(), "usage.json"))
	defer usageStore.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	session := Start(StartParams{
		Usage:   usageStore,
		Request: req,
		Auth:    &auth.RequestAuth{CallerID: "caller-a"},
		Surface: "claude.messages",
		Standard: promptcompat.StandardRequest{
			ResponseModel: "deepseek-v4-pro",
		},
	})
	if session == nil {
		t.Fatal("expected usage-only session")
	}
	session.Success(http.StatusOK, "", "ok", "stop", map[string]any{"input_tokens": 12, "output_tokens": 4})

	entries := usageStore.Summary()
	if len(entries) != 1 {
		t.Fatalf("unexpected usage entries: %+v", entries)
	}
	entry := entries[0]
	if entry.Calls != 1 || entry.PromptTokens != 12 || entry.CompletionTokens != 4 {
		t.Fatalf("unexpected usage entry: %+v", entry)
	}
	if entry.Model != "deepseek-v4-pro" || entry.CallerID != "caller-a" {
		t.Fatalf("unexpected usage dimensions: %+v", entry)
	}
}

func TestDisabledHistorySkipsArchiveAndRetainsUsage(t *testing.T) {
	history := chathistory.New(filepath.Join(t.TempDir(), "history.json"))
	if _, err := history.SetLimit(chathistory.DisabledLimit); err != nil {
		t.Fatal(err)
	}
	usage := usagestats.New(filepath.Join(t.TempDir(), "usage.json"))
	defer usage.Close()
	session := Start(StartParams{
		Store: history, Usage: usage,
		Request: httptest.NewRequest(http.MethodPost, "/v1/messages", nil),
		Auth:    &auth.RequestAuth{CallerID: "usage-only"},
		Standard: promptcompat.StandardRequest{
			ResponseModel: "deepseek-v4.1-flash", FinalPrompt: "keep accounting",
			Messages: []any{map[string]any{"role": "user", "content": "private input"}},
		},
	})
	if session == nil || session.store != nil || len(session.startParams.Messages) != 0 {
		t.Fatalf("disabled history retained an unused archive: %+v", session)
	}
	session.Progress("", "partial")
	session.Stopped("", "partial", "stopped")
	entries := usage.Summary()
	if len(entries) != 1 || entries[0].Calls != 1 || entries[0].PromptTokens == 0 {
		t.Fatalf("usage lost with disabled history: %+v", entries)
	}
	state, err := history.Snapshot()
	if err != nil || len(state.Items) != 0 {
		t.Fatalf("disabled history was written: %+v %v", state, err)
	}
}
