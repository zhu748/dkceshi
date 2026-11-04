package responsehistory

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ds2api/internal/auth"
	"ds2api/internal/chathistory"
	"ds2api/internal/promptcompat"
)

func progressSession(t *testing.T) (*Session, *chathistory.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "history.json")
	store := chathistory.New(path)
	session := Start(StartParams{Store: store, Request: httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil),
		Auth: &auth.RequestAuth{}, Standard: promptcompat.StandardRequest{Stream: true, FinalPrompt: "prompt"}})
	if session == nil || session.entryID == "" {
		t.Fatal("history session missing")
	}
	return session, store, path
}

func TestUnchangedProgressSkipsDiskAndTerminalStateStillPersists(t *testing.T) {
	s, store, _ := progressSession(t)
	s.lastPersist = time.Time{}
	s.Progress("reasoning", "answer")
	before, err := store.Revision()
	if err != nil {
		t.Fatal(err)
	}
	s.lastPersist = time.Time{}
	s.Progress("reasoning", "answer")
	after, err := store.Revision()
	if err != nil || after != before {
		t.Fatalf("unchanged progress rewrote history: before=%d after=%d error=%v", before, after, err)
	}
	s.Success(http.StatusOK, "reasoning", "answer", "stop", nil)
	item, err := store.Get(s.entryID)
	if err != nil || item.Status != "success" || item.CompletedAt == 0 || item.Revision == before {
		t.Fatalf("terminal state was incorrectly coalesced: item=%+v error=%v", item, err)
	}
}

func TestFailedProgressWriteRetriesIdenticalOutput(t *testing.T) {
	s, store, path := progressSession(t)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	s.lastPersist = time.Time{}
	s.Progress("reasoning", "answer")
	if s.lastProgress.valid {
		t.Fatal("failed disk write was accepted as a saved snapshot")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	s.lastPersist = time.Time{}
	s.Progress("reasoning", "answer")
	if !s.lastProgress.valid {
		t.Fatal("identical output did not retry its failed checkpoint")
	}
	restarted := chathistory.New(path)
	item, err := restarted.Get(s.entryID)
	if err != nil || item.Content != "answer" {
		t.Fatalf("retried checkpoint missing on restart: %+v %v", item, err)
	}
	if _, err := store.Revision(); err != nil {
		t.Fatal(err)
	}
}
