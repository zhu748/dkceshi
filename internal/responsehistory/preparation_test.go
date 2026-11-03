package responsehistory

import (
	"errors"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ds2api/internal/auth"
	"ds2api/internal/chathistory"
	dsclient "ds2api/internal/deepseek/client"
	"ds2api/internal/promptcompat"
)

func TestFailedPrepareIsArchivedWithoutRelease(t *testing.T) {
	store := chathistory.New(filepath.Join(t.TempDir(), "history.json"))
	started := time.Now().Add(-3 * time.Second)
	params := StartParams{
		Store: store, Request: httptest.NewRequest("POST", "/v1/chat/completions?__stream_prepare=1", nil),
		Auth: &auth.RequestAuth{CallerID: "caller", AccountID: "owner"}, Surface: "openai.chat_completions", StartedAt: started,
		Standard: promptcompat.StandardRequest{ResponseModel: "deepseek-v4.1-flash", Stream: true, Messages: []any{map[string]any{"role": "user", "content": "continue"}}},
	}
	if Start(params) != nil {
		t.Fatal("successful prepare must still wait for release")
	}
	RecordPreparationError(params, 502, "Failed to upload inline file.", errors.New("upload transport failed"))
	snapshot, err := store.Snapshot()
	if err != nil || len(snapshot.Items) != 1 {
		t.Fatalf("history=%+v err=%v", snapshot, err)
	}
	entry, err := store.Get(snapshot.Items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Status != "error" || entry.StatusCode != 502 || !strings.Contains(entry.Error, "upload transport failed") || entry.ElapsedMs < 3000 || entry.CreatedAt != started.UnixMilli() {
		t.Fatalf("incomplete preparation diagnostic: %+v", entry)
	}
}

func TestFailedPreparePreservesDirectAuthHistoryPolicy(t *testing.T) {
	store := chathistory.New(filepath.Join(t.TempDir(), "history.json"))
	RecordPreparationError(StartParams{Store: store, Request: httptest.NewRequest("POST", "/v1/messages", nil), Auth: &auth.RequestAuth{}}, 401, "invalid token", &dsclient.RequestFailure{Kind: dsclient.FailureDirectUnauthorized})
	snapshot, err := store.Snapshot()
	if err != nil || len(snapshot.Items) != 0 {
		t.Fatalf("history=%+v err=%v", snapshot, err)
	}
}
